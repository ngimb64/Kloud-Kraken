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
	"strings"
	"time"
)

// Reads the passed in PEM encoded certificate file into memory
// and attempts to append it to a newly generated cert pool.
//
// @Parameters
// - caCertFn:  The name of certifcate file to be loaded
//
// @Returns
// - The x509 certificate pool with loaded cert added to it
// - Error if it occurs, otherwise nil on success
//
func CaCertPool(caCertFile string, caCertPemBlock []byte) (*x509.CertPool, error) {
    var caCert []byte
    var err error

    // If the certificate authority certificate file was passed in
    if caCertFile != "" {
        // Read the PEM encoded TLS certificate file
        caCert, err = os.ReadFile(caCertFile)
        if err != nil {
            return nil, err
        }
    // Other wise the raw PEM block bytes are passed in
    } else {
        caCert = caCertPemBlock
    }

    // Create an x509 certificate pool
    certPool := x509.NewCertPool()

    // Attempt to add the loaded certificate to the cert pool
    if ok := certPool.AppendCertsFromPEM(caCert); !ok {
        return nil, errors.New("failed to add certificate to pool")
    }

    return certPool, nil
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
                usableIps = append(usableIps, addr.String())
            case ip.IsGlobalUnicast():
                usableIps = append(usableIps, addr.String())
            }
        }
    }

    return usableIps, nil
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
func GetTlsConfig(cert tls.Certificate, serverPool *x509.CertPool) func(*tls.ClientHelloInfo) (*tls.Config, error) {
    return func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
        // Generate new TLS configuration instance
        cfg := NewTlsConfig(cert, serverPool)
        // Inject the VerifyPeerCertificate callback with access to hello
        cfg.VerifyPeerCertificate = VerifyClientCert(serverPool, hello)
        return cfg, nil
    }
}


// Function for generating a new TLS configuration.
//
// @Parameters
// - cert:  The TLS certificate to be used in config generation
// - serverPool:  The servers TLS certificate pool
//
// @Returns
// - The TLS configuration instance
//
func NewTlsConfig(cert tls.Certificate, serverPool *x509.CertPool) *tls.Config {
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
        ctx:       ctx,
        ready:     make(chan struct{}),
        addr:      address,
        tlsConfig: tlsConfig,
    }
}


// Struct for managing TLS connections
type TlsServer struct {
    ctx   	  context.Context
    ready 	  chan struct{}
    addr      string
    tlsConfig *tls.Config
}

// TlsServer struct method to block ready channel
// until TLS listener setup is complete
func (server *TlsServer) Ready() {
    if server.ready != nil {
        <-server.ready
    }
}

// TlsServer struct method to setup TLS supported TCP listener to handle incoming connections.
//
// @Parameters
// - certPemBlock:  The TLS certificate PEM byte block
// - keyPemBlock:  The TLS key PEM byte block
//
// @Returns
// - The established TLS TCP listener
// - Error if it occurs, otherwise nil on success
//
func (server *TlsServer) SetupTlsListener(certPemBlock []byte, keyPemBlock []byte) (net.Listener, error) {
    // If no address was specified when NewTlsServer was called
    if server.addr == "" {
        server.addr = "localhost:443"
    }

    // Establish raw TCP listener
    listener, err := net.Listen("tcp", server.addr)
    if err != nil {
        return nil, fmt.Errorf("binding to tcp %s: %w", server.addr, err)
    }

    // If the servers context is set
    if server.ctx != nil {
        // Launch routine to catch it when signaled
        go func() {
            <-server.ctx.Done()
            // Close the TLS listener
            _ = listener.Close()
        }()
    }

    // If the TLS config is missing, use basic default config
    if server.tlsConfig == nil {
        server.tlsConfig = &tls.Config{
            CurvePreferences:         []tls.CurveID{tls.CurveP256},
            MinVersion:               tls.VersionTLS12,
            PreferServerCipherSuites: true,
        }
    }

    // If the certificate & key pair is missing from TLS config
    if len(server.tlsConfig.Certificates) == 0 && server.tlsConfig.GetCertificate == nil {
        // Load the certificate & key PEM files
        cert, err := tls.X509KeyPair(certPemBlock, keyPemBlock)
        if err != nil {
            return nil, fmt.Errorf("error loading key pair - %v", err)
        }

        // Add the loaded cert to certificates slice
        server.tlsConfig.Certificates = []tls.Certificate{cert}
    }

    // Create new listener with TLS layer on top of raw TCP listner
    tlsListener := tls.NewListener(listener, server.tlsConfig)
    if server.ready != nil {
        close(server.ready)
    }

    return tlsListener, nil
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
func TlsCertAndKeyGen(name string, hosts string, generateFiles bool) ([]byte, []byte, error) {
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
        return nil, nil, fmt.Errorf("unable to encode the TLS certificate into PEM format")
    }

    // Encode the PKCS key into the PEM file
    keyBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: pkcsKey})
    if keyBytes == nil {
        return nil, nil, fmt.Errorf("unable to encode the PKCS into PEM format")
    }

    // If the TLS certificate and key are to be written as PEM files
    if generateFiles {
        // Create a PEM file to encode for certificate
        certFile, err := os.Create("kloud-kraken-cert.pem")
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
            return nil, nil, fmt.Errorf("no bytes were written to TLS cert PEM file")
        }

        // Close the generated PEM for certificate
        if err := certFile.Close(); err != nil {
            return nil, nil, err
        }

        // Create a PEM file to encode for key
        keyFile, err := os.OpenFile("kloud-kraken-key.pem", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
        if err != nil {
            return nil, nil, err
        }

        // Encode the TLS key into PEM file
        bytesWrote, err = keyFile.Write(keyBytes)
        if err != nil {
            return nil, nil, err
        }

        // If no bytes were written to PEM file
        if bytesWrote < 1 {
            return nil, nil, fmt.Errorf("no bytes were written to TLS key PEM file")
        }

        // Close the generated PEM for certificate
        if err := keyFile.Close(); err != nil {
            return nil, nil, err
        }
    }

    return certBytes, keyBytes, nil
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
