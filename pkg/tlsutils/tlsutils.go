package tlsutils

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Take the passed in PEM certificate bytes and writes them to a file.
//
// @Parameters
//  - certBytes:  The PEM certificate to be written to a file
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func createPemCertFile(certBytes []byte) error {
    if len(certBytes) == 0 {
        return errors.New("no PEM certifcate bytes to write to file")
    }

    // Create a PEM file to encode for certificate
    certFile, err := os.Create("tls-cert.pem")
    if err != nil {
        return err
    }

    defer func() {
        // Close the generated PEM for certificate
        cerr := certFile.Close()
        if cerr != nil {
            err = errors.Join(err, fmt.Errorf("closing created PEM file - %w", cerr))
        }
    }()

    // Write the certificate to PEM file
    bytesWrote, err := certFile.Write(certBytes)
    if err != nil {
        return err
    }

    // If no bytes were written to PEM file
    if bytesWrote < 1 {
        return errors.New("no bytes were written to TLS cert PEM file")
    }

    return nil
}


// Function for generating a new client TLS configuration.
//
// @Parameters
//  - clientPool:  The clients PEM certificate pool with servers cert
//  - serverAddr:  The server IP address to connect to
//
// @Returns
//  - The TLS configuration instance
//
func NewClientTLSConfig(clientPool *x509.CertPool,
                        serverAddr string) *tls.Config {
    return &tls.Config{
        CurvePreferences: []tls.CurveID{tls.CurveP256},
        MinVersion:       tls.VersionTLS13,
        RootCAs:          clientPool,
        ServerName:       serverAddr,
    }
}


// Data structure for managing TLS components
type TlsManager struct {
    certPemBlocks  [][]byte
    CertPool       *x509.CertPool
    mutex          sync.RWMutex
    TlsCertificate tls.Certificate
}

// Adds the TLS cert pem block to management slice.
//
// @Parameters
//  - certPemBytes:  Certificate PEM block to add to slice
//
func (TlsMan *TlsManager) AddCertBytesToPemBlocks(certPemBytes []byte) {
    TlsMan.mutex.Lock()
    defer TlsMan.mutex.Unlock()

    TlsMan.certPemBlocks = append(TlsMan.certPemBlocks, certPemBytes)
}

// Add cert to TLS cert pool.
//
// @Parameters
//  - pemBlock:  The byte PEM certifcate slice to be added to CaCertPool
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (TlsMan *TlsManager) AddCertToPool(pemBlock []byte) error {
    TlsMan.mutex.Lock()
    defer TlsMan.mutex.Unlock()

    // If the TLS cert pool is not set, set it
    if TlsMan.CertPool == nil {
        TlsMan.CertPool = x509.NewCertPool()
    }

    // Append directly into the existing pool
    ok := TlsMan.CertPool.AppendCertsFromPEM(pemBlock)
    if !ok {
        return fmt.Errorf("failed to append new cert PEM block - %q", pemBlock)
    }

    return nil
}

// Generate the TLS certificate from cert & key PEM byte blocks, adds certificate
// to the cert pool, and assigns the certificate & cert pool in TlsManager.
//
// @Parameters
//  - certsToAdd:  variadic length variable of PEM cert files to load and add
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (TlsMan *TlsManager) CertPoolGen(certsToAdd ...string) error {
    TlsMan.mutex.Lock()
    defer TlsMan.mutex.Unlock()

    // If there are PEM cert file passed in, iterate through them
    for _, pemFile := range certsToAdd {
        // Read the PEM encoded TLS certificate file
        pemBlock, err := os.ReadFile(pemFile)
        if err != nil {
            return err
        }

        // Append the read PEM block to byte slice of PEM blocks
        TlsMan.certPemBlocks = append(TlsMan.certPemBlocks, pemBlock)
    }

    // Create an x509 certificate pool
    TlsMan.CertPool = x509.NewCertPool()

    // Iterate through the slice of PEM blocks
    for _, pemBlock := range TlsMan.certPemBlocks {
        // If the PEM block is empty, skip it
        if len(strings.TrimSpace(string(pemBlock))) == 0 {
            continue
        }

        // Attempt to add the loaded certificate to the cert pool
        ok := TlsMan.CertPool.AppendCertsFromPEM(pemBlock)
        if !ok {
            return fmt.Errorf("failed to add certificate PEM block" +
                              " to pool - %q", pemBlock)
        }
    }

    return nil
}

// Generates the TLS certificate & key, saving the result in the TlsMan struct.
//
// @Parameters
//  - orgName:  The organization name to assign to the generated certificate
//  - generateFile:  Toggle for whether PEM file should be generated or not
//  - hostnames:  Variadic length of ip address & hostnames to add to hosts CSV string
//
// @Returns
//  - PEM certificate byte slice
//  - Error if it occurs, otherwise nil on success
//
func (TlsMan *TlsManager) PemCertAndKeyGen(orgName string, generateFile bool,
                                           hostnames ...string) (
                                           []byte, error) {
    if len(hostnames) < 1 {
        return nil, fmt.Errorf("no hostnames or IP addresses present")
    }

    // Join slice of hostnames in comma,separated,format
    hostsCsv := strings.Join(hostnames, ",")

    // Create a cryptographically secure random 128 bit integer
    serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
    if err != nil {
        return nil, err
    }

    // Get the time for certifcate generation
    notBefore := time.Now().Add(-15 * time.Minute)
    // Set up the TLS certificate settings
    template := x509.Certificate{
        SerialNumber: serial,
        Subject: pkix.Name{
            Organization: []string{orgName},
        },
        NotBefore:   notBefore,
        NotAfter:    notBefore.Add(1 * 365 * 24 * time.Hour),
        KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
        ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
        BasicConstraintsValid: true,
        IsCA: false,
    }

    // Split the comma-separated host list and iterate through it
    for host := range strings.SplitSeq(hostsCsv, ",") {
        // Parse string as IP address
        ip := net.ParseIP(host)
        // If the entry is an ip address
        if ip != nil {
            template.IPAddresses = append(template.IPAddresses, ip)
        // If the entry is a hostname or localhost
        } else {
            template.DNSNames = append(template.DNSNames, host)
        }
    }

    // Create the PEM certificate and key
    certPemBlock, err := TlsMan.createX509Cert(&template)
    if err != nil {
        return nil, err
    }

    // If the PEM certificate is to be written as files
    if generateFile {
        err = createPemCertFile(certPemBlock)
        if err != nil {
            return nil, err
        }
    }

    return certPemBlock, nil
}

// Generate the PEM certificate and key in memory and returns the result.
//
// @Parameters
//  - template:  The x509 certificate template
//
// @Returns
//  - PEM certificate byte slice
//  - Error if it occurs, otherwise nil on success
//
func (TlsMan *TlsManager) createX509Cert(template *x509.Certificate) (
                                         []byte, error) {
    // Generate ECDSA key for cert and key generation
    ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
    if err != nil {
        return nil, err
    }

    // Generate a x509 cerfiticate with ECDSA key
    cert, err := x509.CreateCertificate(rand.Reader, template, template,
                                        &ecdsaKey.PublicKey, ecdsaKey)
    if err != nil {
        return nil, err
    }

    // Convert private key to PKCS
    ecKeyBytes, err := x509.MarshalECPrivateKey(ecdsaKey)
    if err != nil {
        return nil, err
    }

    // Encode the TLS certificate into the PEM file
    certBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert})
    if certBytes == nil {
        return nil, errors.New("unable to encode the TLS certificate into PEM format")
    }

    // Encode the PKCS key into the PEM file
    keyBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: ecKeyBytes})
    if keyBytes == nil {
        return nil, errors.New("unable to encode the PKCS into PEM format")
    }

    TlsMan.mutex.Lock()
    defer TlsMan.mutex.Unlock()

    // Generate certificate base on certificate & key PEM blocks
    TlsMan.TlsCertificate, err = tls.X509KeyPair(certBytes, keyBytes)
    if err != nil {
        return nil, err
    }

    return certBytes, nil
}

// Creates TLS x509 certificate and a cert pool which are used to setup the TLS
// configuration instance. After a TLS listener is established and returned.
//
// @Parameters
//  - cert:  The TLS certificate to use
//  - ctx:  The context handler for inner raw TCP socket
//  - listenIp:  The IP address of the network interface of TLS listener
//  - listenPort:  Port that TLS listener will attempt to be established on
//  - listener:  The raw TCP listener to use, passing in nil will result in
//              one being created
//
// @Returns
//  - The established TLS listener
//  - Error if it occurs, otherwise nil on success
//
func SetupTlsListenerHandler(cert tls.Certificate, ctx context.Context,
                             listenIp string, listenPort int,
                             listener net.Listener) (
                             net.Listener, error) {
    // Create a TLS configuration instance
    tlsConfig := &tls.Config{
        Certificates:     []tls.Certificate{cert},
        ClientAuth:   	  tls.NoClientCert,
        CurvePreferences: []tls.CurveID{tls.CurveP256},
        MinVersion:       tls.VersionTLS13,
    }

    // Format host address like <ip_address>:<port>
    address := net.JoinHostPort(listenIp, strconv.Itoa(listenPort))
    var err error

    // If no active listener was passed in
    if listener == nil {
        // Establish raw TCP listener
        listener, err = net.Listen("tcp", address)
        if err != nil {
            return nil, fmt.Errorf("binding to tcp %q - %w", address, err)
        }
    }

    // If the servers context is set
    if ctx != nil {
        // Launch routine to close listener when signaled
        go func() {
            <-ctx.Done()
            _ = listener.Close()
        }()
    }

    // Create new listener with TLS layer on top of raw TCP listner
    tlsListener := tls.NewListener(listener, tlsConfig)
    return tlsListener, nil
}
