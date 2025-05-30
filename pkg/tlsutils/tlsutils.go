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
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// HTTP shared client (reuses connections) with global timeout
var Client = &http.Client{Timeout: 5*time.Minute}
// Pre-compile IPv4/IPv6 regex once
var ReIpAddr = regexp.MustCompile(
    `\b(?:\d{1,3}\.){3}\d{1,3}\b|` +  // IPv4
    `\b(?:[0-9A-Fa-f]{1,4}:){2,7}[0-9A-Fa-f]{1,4}\b`,  // IPv6 (simple form)
)


// Attempts a GET request to retrieve IP data from passed in API URL.
//
// @Parameters
// - ctx:  The context handler for the request
// - cancel:  The cancel function for the context handler
// - url:  The url of the API to attempt to retrieve IP data
//
// @Returns
// - The retrieved IP data from the GET request
// - Error if it occurs, otherwise nil on success
//
func GetIpData(ctx context.Context, cancel context.CancelFunc, url string) (
               []byte, error) {
    // Cancel request context on local exit
    defer cancel()
    // Initialize HTTP GET request
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return []byte(""), err
    }

    // Send HTTP GET request
    response, err := Client.Do(req)
    if err != nil {
        return []byte(""), err
    }

    // Read the response data of the request
    data, err := io.ReadAll(response.Body)
    response.Body.Close()
    if err != nil {
        return []byte(""), err
    }

    return data, nil
}


// GetPublicIP tries each endpoint in turn until one returns a valid IPv4/6 string.
//
// @Returns
// - A slice of string IP address retrieved from APIs
// - Error if it occurs, otherwise nil on success
//
func GetPublicIps() ([]string, error) {
    var ipAddrs []string
    uniqueAddrs := make(map[string]struct{})
    // list of public‐IP endpoints to try, in order
    endpoints := []string{"https://api.ipify.org", "https://ifconfig.me/ip",
                          "https://checkip.amazonaws.com", "https://icanhazip.com"}

    // Iterate through list of IP API enpoints
    for _, url := range endpoints {
        // Create a fresh 5s context for each request
        ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
        // Execute GET request to retrieve IP data
        data, err := GetIpData(ctx, cancel, url)
        if err != nil {
            continue
        }

        // Convert data to string and remove any outer whitespace
        textData := strings.TrimSpace(string(data))
        // Regex search for all IP address
        matches := ReIpAddr.FindAllString(textData, -1)

        // Iterate through matched IP addresses
        for _, match := range matches {
            // Check to see if IP has been matched already
            _, exists := uniqueAddrs[match]
            // If IP does not exist in map
            if !exists {
                // Add it to map and resulting slice
                uniqueAddrs[match] = struct{}{}
                ipAddrs = append(ipAddrs, match)
            }
        }
    }

    // If no public IPs were discovered
    if len(ipAddrs) == 0 {
        return nil, errors.New("could not retrieve public IP from any APIs")
    }

    return ipAddrs, nil
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

            // If the IP address is public or private
            if ip.IsGlobalUnicast() || ip.IsPrivate() {
                // Add it to the usable IPs slice
                usableIps = append(usableIps, ip.String())
            }
        }
    }

    return usableIps, nil
}


// Function for generating a new client TLS configuration.
//
// @Parameters
// - clientPool:  The clients PEM certificate pool with servers cert
// - serverAddr:  The server IP address to connect to
//
// @Returns
// - The TLS configuration instance
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
    Addr            string
    CaCertPemBlocks [][]byte
    CaCertPool      *x509.CertPool
    CertPemBlock    []byte
    Ctx   	        context.Context
    KeyPemBlock     []byte
    TlsCertificate  tls.Certificate
    TlsConfig       *tls.Config
}

// Add the cert to TlsManager CaCertPool
//
// @Parameters
// - pemBlock:  The byte PEM certifcate slice to be added to CaCertPool
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
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

// Generate the TLS certificate from cert & key PEM byte blocks, adds certificate
// to the cert pool, and assigns the certificate & cert pool in TlsManager.
//
// @Parameters
// - tlsCertPem:  The cert PEM bytes used for certificate generation
// - tlsKeyPem:  The key PEM bytes used for certificate generation
// - caCertPemBlocks:  The slice of byte slices used to store CA PEM blocks
// - certsToAdd:  variadic length variable of PEM cert files to load and add
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func (TlsMan *TlsManager) CertGenAndPool(tlsCertPem []byte, tlsKeyPem []byte,
                                        caCertPemBlocks [][]byte,
                                        certsToAdd ...string) error {
    // Generate certificate base on certificate & key PEM blocks
    cert, err := tls.X509KeyPair(tlsCertPem, tlsKeyPem)
    if err != nil {
        return err
    }

    // Create server x509 certificate pool
    certPool, err := TlsMan.CaCertPoolGen(caCertPemBlocks, certsToAdd...)
    if err != nil {
        return err
    }

    TlsMan.TlsCertificate = cert
    TlsMan.CaCertPool = certPool

    return nil
}

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
func (TlsMan *TlsManager) CaCertPoolGen(caCertPemBlocks [][]byte, caCertPemFiles ...string) (
                                        *x509.CertPool, error) {
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

// Generates the TLS certificate & key, saving the result in the TlsMan struct.
//
// @Parameters
// - orgName:  The organization name to assign to the generated certificate
// - testMode:  boolean toggle for whether PEM file should be generated or not
// - hostnames:  variadic length variable of ip address and hostnames to add to hosts CSV string
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func (TlsMan *TlsManager) PemCertAndKeyGenHandler(orgName string, testMode bool,
                                                  hostnames ...string) error {
    // Add the localhost to CA hosts list
    hosts := "localhost"

    // Iterate through any passed in host names and add them to hosts CSV string
    for _, host := range hostnames {
        hosts += ("," + host)
    }

    // Get available usable public/private IP's assigned to network interfaces
    ipAddrs, err := GetUsableIps()
    if err != nil {
        return err
    }

    // Iterate through the list IP's and add them to CSV string
    for _, ipAddr := range ipAddrs {
        hosts += ("," + ipAddr)
    }

    // Generate the TLS certificate/key and save them in app config
    TlsMan.CertPemBlock,
    TlsMan.KeyPemBlock, err = TlsMan.PemCertAndKeyGen(orgName, hosts, testMode)
    if err != nil {
        return err
    }

    return nil
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
// @Returns
// - PEM byte block for TLS certificate
// - PEM byte block for TLS key
// - Error if it occurs, otherwise nil on success
//
func (TlsMan *TlsManager) PemCertAndKeyGen(name string, hosts string, generateFiles bool) (
                                           []byte, []byte, error) {
    // Create a cryptographically secure random 128 bit integer
    serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
    if err != nil {
        return nil, nil, err
    }

    // Get the time for certifcate generation
    notBefore := time.Now().Add(-15 * time.Minute)
    // Set up the TLS certificate settings
    template := x509.Certificate{
        SerialNumber: serial,
        Subject: pkix.Name{
            Organization: []string{name},
        },
        NotBefore:   notBefore,
        NotAfter:    notBefore.Add(1 * 365 * 24 * time.Hour),
        KeyUsage:    x509.KeyUsageDigitalSignature,
        ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
        BasicConstraintsValid: true,
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

    // Create the PEM certificate and key
    certBytes, keyBytes, err := TlsMan.CreatePemCertAndKey(&template)
    if err != nil {
        return nil, nil, err
    }

    // If the PEM certificate and key are to be written as files
    if generateFiles {
        err = TlsMan.CreatePemCertFile(certBytes)
        if err != nil {
            return nil, nil, err
        }
    }

    return certBytes, keyBytes, nil
}

// Generate the PEM certificate and key in memory and returns the result.
//
// @Parameters
// - template:  The x509 certificate template
//
// @Returns
// - The certificate PEM bytes
// - The key PEM bytes
// - Error if it occurs, otherwise nil on success
//
func (TlsMan *TlsManager) CreatePemCertAndKey(template *x509.Certificate) (
                                              []byte, []byte, error) {
    // Generate ECDSA key for cert and key generation
    ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
    if err != nil {
        return nil, nil, err
    }

    // Generate a x509 cerfiticate with ECDSA key
    cert, err := x509.CreateCertificate(rand.Reader, template, template,
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

    return certBytes, keyBytes, nil
}

// Take the passed in PEM certificate bytes and writes them to a file.
//
// @Parameters
// - certBytes:  The PEM certificate to be written to a file
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func (TlsMan *TlsManager) CreatePemCertFile(certBytes []byte) error {
    // Create a PEM file to encode for certificate
    certFile, err := os.Create("tls-cert.pem")
    if err != nil {
        return err
    }

    // Write the certificate to PEM file
    bytesWrote, err := certFile.Write(certBytes)
    if err != nil {
        return err
    }

    // If no bytes were written to PEM file
    if bytesWrote < 1 {
        return errors.New("no bytes were written to TLS cert PEM file")
    }

    // Close the generated PEM for certificate
    err = certFile.Close()
    if err != nil {
        return err
    }

    return nil
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
func (TlsMan *TlsManager) SetupTlsListenerHandler(cert tls.Certificate, certPool *x509.CertPool,
                                                  ctx context.Context, listenIp string,
                                                  listenPort int, listener net.Listener) (
                                                  net.Listener, error) {
    // Create a TLS configuration instance
    tlsConfig := &tls.Config{
        Certificates:       []tls.Certificate{cert},
        GetConfigForClient: TlsMan.GetServerTlsConfig(cert, certPool),
    }

    // Format listener address with port
    listenerAddr := listenIp + ":" + strconv.Itoa(listenPort)
    // Set needed struct members for setting up TLS listener
    TlsMan.Addr = listenerAddr
    TlsMan.Ctx = ctx
    TlsMan.TlsConfig = tlsConfig
    // Setup TLS listener from server instance
    tlsListener, err := TlsMan.SetupTlsListener(listener)
    if err != nil {
        return nil, err
    }

    return tlsListener, nil
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
func (TlsMan *TlsManager) GetServerTlsConfig(cert tls.Certificate, serverPool *x509.CertPool) func(
                                             *tls.ClientHelloInfo) (*tls.Config, error) {
    return func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
        // Generate new TLS configuration instance
        cfg := TlsMan.NewServerTlsConfig(cert)
        // Inject the VerifyPeerCertificate callback with access to hello
        cfg.VerifyPeerCertificate = TlsMan.VerifyClientCert(serverPool, hello)
        return cfg, nil
    }
}

// Function for generating a new server listener TLS configuration.
//
// @Parameters
// - cert:  The TLS certificate to be used in config generation
//
// @Returns
// - The TLS configuration instance
//
func (TlsMan *TlsManager) NewServerTlsConfig(cert tls.Certificate) *tls.Config {
    return &tls.Config{
        Certificates: 			  []tls.Certificate{cert},
        ClientAuth:   			  tls.NoClientCert,
        CurvePreferences: 		  []tls.CurveID{tls.CurveP256},
        MinVersion:         	  tls.VersionTLS13,
        PreferServerCipherSuites: true,
    }
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
func (TlsMan *TlsManager) VerifyClientCert(serverPool *x509.CertPool,
                                           hello *tls.ClientHelloInfo) func(
                                           rawCerts [][]byte,
                                           verifiedChains [][]*x509.Certificate) error {
    return func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
        // Verify x509 certificate options
        opts := x509.VerifyOptions{
            KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
            Roots:     serverPool,
        }

        // Figure out how the peer called us
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

// TlsServer struct method to setup TLS supported TCP listener to handle incoming connections.
//
// @Parameters
// - listener:  Established raw TCP socket listener, if nil one is created
//
// @Returns
// - The established TLS TCP listener
// - Error if it occurs, otherwise nil on success
//
func (TlsMan *TlsManager) SetupTlsListener(listener net.Listener) (net.Listener, error) {
    var err error

    // If no active listener was passed in
    if listener == nil {
        // Establish raw TCP listener
        listener, err = net.Listen("tcp", TlsMan.Addr)
        if err != nil {
            return nil, fmt.Errorf("binding to tcp %s: %w", TlsMan.Addr, err)
        }
    }

    // If the servers context is set
    if TlsMan.Ctx != nil {
        // Launch routine to catch it when signaled
        go func() {
            <-TlsMan.Ctx.Done()
            // Close the TLS listener
            _ = listener.Close()
        }()
    }

    // Create new listener with TLS layer on top of raw TCP listner
    tlsListener := tls.NewListener(listener, TlsMan.TlsConfig)

    return tlsListener, nil
}
