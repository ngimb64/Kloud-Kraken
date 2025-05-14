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
	"time"
)

// Reads the passed in PEM encoded certificate file into memory
// and attempts to append it to a newly generated cert pool.
//
// @Parameters
// - caCertPemBlocks:  The byte PEM block to be used instead of file
// - caCertFiles:  Variadic length slice of PEM files to be loaded into caCertPemBlocks
//
// @Returns
// - The x509 certificate pool with loaded cert added to it
// - Error if it occurs, otherwise nil on success
//
func CaCertPool(caCertPemBlocks [][]byte, caCertPemFiles ...string) (*x509.CertPool, error) {
    // If there are PEM cert file passed in, iterate through them
    for _, pemFile := range caCertPemFiles {
        // Read the PEM encoded TLS certificate file
        pemBlock, err := os.ReadFile(pemFile)
        if err != nil {
            return nil, err
        }

        // Append the read PEM block to byte slice of PEM blocks
        caCertPemBlocks = append(caCertPemBlocks, pemBlock)
    }

    // Create an x509 certificate pool
    certPool := x509.NewCertPool()

    // Iterate through the slice of PEM blocks
    for _, pemBlock := range caCertPemBlocks {
        // Attempt to add the loaded certificate to the cert pool
        ok := certPool.AppendCertsFromPEM(pemBlock)
        if !ok {
            return nil, errors.New("failed to add certificate to pool")
        }
    }

    return certPool, nil
}


// Generate the TLS certificate from cert & key PEM byte blocks, then adds the
// certificate to the cert pool.
//
// @Parameters
// - tlsCertPem:  The cert PEM bytes used for certificate generation
// - tlsKeyPem:  The key PEM bytes used for certificate generation
// - caCertPemBlocks:  The slice of byte slices used to store CA PEM blocks
// - certsToAdd:  variadic length variable of PEM cert files to load and add
//
// @Returns
// - The generated TLS certificate
// - The generated TLS cert pool
// - Error if it occurs, otherwise nil on success
//
func CertGenAndPool(tlsCertPem []byte, tlsKeyPem []byte, caCertPemBlocks [][]byte,
                    certsToAdd ...string) (tls.Certificate, *x509.CertPool, error) {
    // Generate certificate base on certificate & key PEM blocks
    cert, err := tls.X509KeyPair(tlsCertPem, tlsKeyPem)
    if err != nil {
        return tls.Certificate{}, nil, err
    }

    // Create server x509 certificate pool
    certPool, err := CaCertPool(caCertPemBlocks, certsToAdd...)
    if err != nil {
        return tls.Certificate{}, nil, err
    }

    return cert, certPool, nil
}


// Function for handling the TLS config generation and client verification.
//
// @Parameters
// - cert:  The TLS certificate to be used in config generation
// - serverPool:  The servers TLS certificate pool
//
// @Returns
// - function that returns the TLS config and errors if any occur
//
func GetServerTlsConfig(cert tls.Certificate, serverPool *x509.CertPool) func(*tls.ClientHelloInfo) (*tls.Config, error) {
    return func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
        // Generate new TLS configuration instance
        cfg := NewServerTlsConfig(cert, serverPool)
        // Inject the VerifyPeerCertificate callback with access to hello
        cfg.VerifyPeerCertificate = VerifyClientCert(serverPool, hello)
        return cfg, nil
    }
}


// Get the assigned valid public and private IP address assigned
// to network interfaces.
//
// @Returns
// - string slice of usable IP addresses
// - Error if it occurs, otherwise nil on success
//
func GetUsableIps() ([]string, error) {
    usableIps := []string{}

    // Get a list of all interfaces on system
    ifaces, err := net.Interfaces()
    if err != nil {
        return nil, err
    }

    // Iterate through list of retrieved interfaces
    for _, iface := range ifaces {
        // Skip if interface is down or a loopback
        if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
            continue
        }

        // Get all addresses assigned to current interface
        addrs, err := iface.Addrs()
        if err != nil {
            continue
        }

        // Iterate through the retrieved addresses
        for _, addr := range addrs {
            // Parse network CIDR based on IP address
            ip, _, err := net.ParseCIDR(addr.String())
            if err != nil {
                continue
            }

            // Check to see if IP address is a usable public or private address,
            // add it to usable IPs slice if it is
            switch {
            case ip.IsPrivate():
                usableIps = append(usableIps, ip.String())
            case ip.IsGlobalUnicast():
                usableIps = append(usableIps, ip.String())
            }
        }
    }

    return usableIps, nil
}


// Function for generating a new client TLS configuration.
//
// @Parameters
// - cert:  The TLS certificate to be used in config generation
// - certPool:  The clients PEM certificate pool
//
// @Returns
// - The TLS configuration instance
//
func NewClientTLSConfig(clientCert tls.Certificate, clientPool *x509.CertPool) *tls.Config {
    return &tls.Config{
        Certificates:       []tls.Certificate{clientCert},
        CurvePreferences:   []tls.CurveID{tls.CurveP256},
        MinVersion:         tls.VersionTLS13,
        RootCAs:            clientPool,
    }
}


// Function for generating a new server listener TLS configuration.
//
// @Parameters
// - cert:  The TLS certificate to be used in config generation
// - serverPool:  The servers PEM certificate pool
//
// @Returns
// - The TLS configuration instance
//
func NewServerTlsConfig(cert tls.Certificate, serverPool *x509.CertPool) *tls.Config {
    return &tls.Config{
        Certificates: 			  []tls.Certificate{cert},
        ClientAuth:   			  tls.RequireAndVerifyClientCert,
        ClientCAs:    			  serverPool,
        CurvePreferences: 		  []tls.CurveID{tls.CurveP256},
        MinVersion:         	  tls.VersionTLS13,
        PreferServerCipherSuites: true,
    }
}


// Creates new TLS server instance to accepting incomming connections
//
// @Parameters
// - ctx:  The context handler for mananging listeneer connection
// - address:  The listener address
// - tlsConfig:  The TLS configuration instance
//
// @Returns
// - The created TLS server instance
//
func NewTlsServer(ctx context.Context, address string,
                  tlsConfig *tls.Config) *TlsServer {
    return &TlsServer{
        Ctx:       ctx,
        Ready:     make(chan struct{}),
        Addr:      address,
        TlsConfig: tlsConfig,
    }
}


// Generates a TLS certficate and key converted to PEM format, if generateFiles boolean is
// toggled then PEM cert and key will be written as files in addition to returned in memory.
//
// @Params
// - name:  name of organization to put on the certificate
// - hosts:  A comma-separated string with the IP addresses and DNS names used by clients
//           to be able to connect with the server that generated it
// - generateFiles:  Toggle for specifying whether PEM files shoud be generated
//
// @ Returns
// - PEM byte block for TLS certificate
// - PEM byte block for TLS key
// - Error if it occurs, otherwise nil on success
//
func PemCertAndKeyGen(name string, hosts string, generateFiles bool) ([]byte, []byte, error) {
    // Create a cryptographically secure random 128 bit integer
    serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
    if err != nil {
        return nil, nil, err
    }

    // Get the time for certifcate generation
    notBefore := time.Now()
    // Set up the TLS certificate settings
    template := x509.Certificate{
        SerialNumber: serial,
        Subject: pkix.Name{
            Organization: []string{name},
        },
        NotBefore: notBefore,
        NotAfter:  notBefore.Add(1 * 365 * 24 * time.Hour),
        KeyUsage: x509.KeyUsageKeyEncipherment |
            x509.KeyUsageDigitalSignature |
            x509.KeyUsageCertSign,
        ExtKeyUsage: []x509.ExtKeyUsage{
            x509.ExtKeyUsageServerAuth,
            x509.ExtKeyUsageClientAuth,
        },
        BasicConstraintsValid: true,
        IsCA:                  true,
    }

    // Split the comma-separated host list and iterate through it
    for _, h := range strings.Split(hosts, ",") {
        // If the entry is an ip address
        if ip := net.ParseIP(h); ip != nil {
            template.IPAddresses = append(template.IPAddresses, ip)
        // If the entry is a hostname or localhost
        } else {
            template.DNSNames = append(template.DNSNames, h)
        }
    }

    // Generate ECDSA key for cert and key generation
    ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
    if err != nil {
        return nil, nil, err
    }

    // Generate a x509 cerfiticate with ECDSA key
    cert, err := x509.CreateCertificate(rand.Reader, &template, &template,
                                        &ecdsaKey.PublicKey, ecdsaKey)
    if err != nil {
        return nil, nil, err
    }

    // Convert private key to PKCS
    pkcsKey, err := x509.MarshalPKCS8PrivateKey(ecdsaKey)
    if err != nil {
        return nil, nil, err
    }

    // Encode the TLS certificate into the PEM file
    certBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert})
    if certBytes == nil {
        return nil, nil, errors.New("unable to encode the TLS certificate into PEM format")
    }

    // Encode the PKCS key into the PEM file
    keyBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: pkcsKey})
    if keyBytes == nil {
        return nil, nil, errors.New("unable to encode the PKCS into PEM format")
    }

    // If the TLS certificate and key are to be written as PEM files
    if generateFiles {
        // Create a PEM file to encode for certificate
        certFile, err := os.Create("tls-cert.pem")
        if err != nil {
            return nil, nil, err
        }

        // Write the certificate to PEM file
        bytesWrote, err := certFile.Write(certBytes)
        if err != nil {
            return nil, nil, err
        }

        // If no bytes were written to PEM file
        if bytesWrote < 1 {
            return nil, nil, errors.New("no bytes were written to TLS cert PEM file")
        }

        // Close the generated PEM for certificate
        if err := certFile.Close(); err != nil {
            return nil, nil, err
        }
    }

    return certBytes, keyBytes, nil
}


// Generates the TLS certificate & key, saving the result in the appConifg struct.
//
// @Parameters
// - orgName:  The organization name to assign to the generated certificate
// - testMode:  boolean toggle for whether PEM file should be generated or not
// - hostnames:  variadic length variable of ip address and hostnames to add to hosts CSV string
//
// @Returns
// - Generated certificate PEM byte block
// - Generated key PEM byte block
// - Error if it occurs, otherwise nil on success
//
func PemCertAndKeyGenHandler(orgName string, testMode bool,
                             hostnames ...string) ([]byte, []byte, error) {
    // Add the localhost to CA hosts list
    hosts := "localhost"

    // Iterate through any passed in host names and add them to hosts CSV string
    for _, host := range hostnames {
        hosts += ("," + host)
    }

    // Get available usable public/private IP's assigned to network interfaces
    ipAddrs, err := GetUsableIps()
    if err != nil {
        return nil, nil, err
    }

    // Iterate through the list IP's and add them to CSV string
    for _, ipAddr := range ipAddrs {
        hosts += ("," + ipAddr)
    }

    // Generate the TLS certificate/key and save them in app config
    certPem, keyPem, err := PemCertAndKeyGen(orgName, hosts, testMode)
    if err != nil {
        return nil, nil, err
    }

    return certPem, keyPem, nil
}


// Struct for managing TLS connections
type TlsServer struct {
    Ctx   	  context.Context
    Addr      string
    TlsConfig *tls.Config
}

// TlsServer struct method to setup TLS supported TCP listener to handle incoming connections.
//
// @Returns
// - The established TLS TCP listener
// - Error if it occurs, otherwise nil on success
//
func (server *TlsServer) SetupTlsListener(listener net.Listener) (net.Listener, error) {
    var err error

    // If no active listener was passed in
    if listener == nil {
        // If no address was specified when NewTlsServer was called
        if server.Addr == "" {
            server.Addr = "localhost:443"
        }

        // Establish raw TCP listener
        listener, err = net.Listen("tcp", server.Addr)
        if err != nil {
            return nil, fmt.Errorf("binding to tcp %s: %w", server.Addr, err)
        }
    }

    // If the servers context is set
    if server.Ctx != nil {
        // Launch routine to catch it when signaled
        go func() {
            <-server.Ctx.Done()
            // Close the TLS listener
            _ = listener.Close()
        }()
    }

    // Create new listener with TLS layer on top of raw TCP listner
    tlsListener := tls.NewListener(listener, server.TlsConfig)

    return tlsListener, nil
}


// Creates TLS x509 certificate and a cert pool which are used to setup the TLS
// configuration instance. After a TLS listener is established and returned.
//
// @Parameters
// - cert:  The TLS certificate to use
// - certPool:  The PEM cert pool to use
// - ctx:  The context handler for inner raw TCP socket
// - listenIp:  The IP address of the network interface of TLS listener
// - listenPort:  Port that TLS listener will attempt to be established on
// - listener:  The raw TCP listener to use, passing in nil will result in
//              one being created
//
// @Returns
// - The established TLS listener
// - Error if it occurs, otherwise nil on success
//
func SetupTlsListenerHandler(cert tls.Certificate, certPool *x509.CertPool,
                             ctx context.Context, listenIp string, listenPort int,
                             listener net.Listener) (net.Listener, error) {
    // Create a TLS configuarion instance
    tlsConfig := &tls.Config{
        Certificates:       []tls.Certificate{cert},
        GetConfigForClient: GetServerTlsConfig(cert, certPool),
    }

    // Format listener address with port
    listenerAddr := listenIp + ":" + strconv.Itoa(listenPort)
    // Create a TLS server instance
    tlsServer := NewTlsServer(ctx, listenerAddr, tlsConfig)
    // Setup TLS listener from server instance
    tlsListener, err := tlsServer.SetupTlsListener(listener)
    if err != nil {
        return nil, err
    }

    return tlsListener, nil
}


// Data structure for managing TLS components
type TlsManager struct {
    CertPemBlock    []byte
    KeyPemBlock     []byte
    CaCertPemBlocks [][]byte
    TlsCertificate  tls.Certificate
    CaCertPool      *x509.CertPool
}

// Method for adding cert to TlsManager CaCertPool
func (TlsMan *TlsManager) AddCACert(pemBlock []byte) error {
    // Add it to your slice for record-keeping
    TlsMan.CaCertPemBlocks = append(TlsMan.CaCertPemBlocks, pemBlock)

    // Append directly into the existing pool
    ok := TlsMan.CaCertPool.AppendCertsFromPEM(pemBlock)
    if !ok {
        return fmt.Errorf("failed to append new CA cert PEM")
    }

    return nil
}


// Function for handling the TLS config generation and client verification
//
// @Parameters
// - serverPool:  The servers TLS certificate pool
// - hello:  TLS hello function pointer
//
// @Returns
// - function that returns errors if any occur
//
func VerifyClientCert(serverPool *x509.CertPool, hello *tls.ClientHelloInfo) func(rawCerts [][]byte,
    verifiedChains [][]*x509.Certificate) error {
    return func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
        // Verify x509 certificate options
        opts := x509.VerifyOptions{
            KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
            Roots:     serverPool,
        }

        // figure out how the peer called us
        ip := strings.Split(hello.Conn.RemoteAddr().String(), ":")[0]
        // Lookup the hostname based on the on IP address
        hostnames, err := net.LookupAddr(ip)
        if err != nil {
            return fmt.Errorf("error looking up IP address - %w", err)
        }

        // Append the IP to the hostname lookup result
        hostnames = append(hostnames, ip)

        // Iterate through the slice of x509 verified chains
        for _, chain := range verifiedChains {
            // Set intermediates certificate pool
            opts.Intermediates = x509.NewCertPool()

            // Add certs to intermediate cert pool
            for _, cert := range chain[1:] {
                opts.Intermediates.AddCert(cert)
            }

            // Iterate through the hostanmes
            for _, hostname := range hostnames {
                // Set in DNS names slice
                opts.DNSName = hostname
                // If the current link in chain is empty
                if _, err := chain[0].Verify(opts); err == nil {
                    return nil
                }
            }
        }

        return errors.New("client authentication failed")
    }
}
