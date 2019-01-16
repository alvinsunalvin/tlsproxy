package main

import (
	"crypto/tls"
	"flag"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/keyman"
	"github.com/oxtoacart/bpool"

	"github.com/getlantern/tlsproxy"
)

var (
	log = golog.LoggerFor("tlsproxy")

	mode            = flag.String("mode", "server", "Mode.  server = listen for TLS connections, client = listen for plain text connections")
	hostname        = flag.String("hostname", "", "Hostname to use for TLS. If not supplied, will auto-detect hostname")
	listenAddr      = flag.String("listen-addr", ":6380", "Address at which to listen for incoming connections")
	forwardAddr     = flag.String("forward-addr", "localhost:6379", "Address to which to forward connections")
	keepAlivePeriod = flag.Duration("keepaliveperiod", 2*time.Minute, "Period for sending tcp keepalives")
	pkfile          = flag.String("pkfile", "pk.pem", "File containing private key for this proxy")
	certfile        = flag.String("certfile", "cert.pem", "File containing the certificate for this proxy")
	cafile          = flag.String("cafile", "cert.pem", "File containing the certificate authority (or just certificate) with which to verify the remote end's identity")
	pprofAddr       = flag.String("pprofaddr", "localhost:4000", "pprof address to listen on, not activate pprof if empty")
	help            = flag.Bool("help", false, "Get usage help")

	buffers = bpool.NewBytePool(25000, 32768)
)

func main() {
	flag.Parse()
	if *help {
		flag.Usage()
		os.Exit(0)
	}

	if *pprofAddr != "" {
		go func() {
			log.Debugf("Starting pprof page at http://%s/debug/pprof", *pprofAddr)
			if err := http.ListenAndServe(*pprofAddr, nil); err != nil {
				log.Error(err)
			}
		}()
	}

	hostname := *hostname
	if hostname == "" {
		_hostname, err := os.Hostname()
		if err == nil {
			hostname = _hostname
		}
	}
	if hostname == "" {
		hostname = "localhost"
	}

	log.Debugf("Mode: %v", *mode)
	log.Debugf("Hostname: %v", hostname)
	log.Debugf("Forwarding to: %v", *forwardAddr)
	log.Debugf("TCP KeepAlive Period: %v", *keepAlivePeriod)

	tlsConfig := &tls.Config{}

	if *pkfile != "" && *certfile != "" {
		cert, err := keyman.KeyPairFor(hostname, "getlantern.org", *pkfile, *certfile)
		if err != nil {
			log.Fatalf("Unable to load keypair: %v", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	ca, err := keyman.LoadCertificateFromFile(*cafile)
	if err != nil {
		log.Fatalf("Unable to load ca certificate: %v", err)
	}
	pool := ca.PoolContainingCert()
	tlsConfig.RootCAs = pool

	l, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		log.Fatalf("Unable to listen at %v: %v", *listenAddr, err)
	}

	switch *mode {
	case "server":
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		tlsConfig.ClientCAs = pool
		tlsproxy.RunServer(l, *forwardAddr, *keepAlivePeriod, tlsConfig)
	case "client":
		tlsproxy.RunClient(l, *forwardAddr, *keepAlivePeriod, tlsConfig)
	default:
		log.Fatalf("Unknown mode: %v", *mode)
	}
}
