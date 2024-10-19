package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	log "github.com/sirupsen/logrus"
	"github.com/trustpilot/beat-exporter/collector"
)

const (
	serviceName = "beat_exporter"
)

func main() {
	var (
		listenAddress = flag.String("web.listen-address", ":9479", "Address to listen on for web interface and telemetry.")
		tlsCertFile   = flag.String("tls.certfile", "", "TLS cert file for HTTPS.")
		tlsKeyFile    = flag.String("tls.keyfile", "", "TLS key file for HTTPS.")
		metricsPath   = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
		beatURIs      = flag.String("beat.uris", "http://localhost:5066", "Comma-separated list of HTTP API addresses of Beats.")
		beatTimeout   = flag.Duration("beat.timeout", 10*time.Second, "Timeout for trying to get stats from Beats.")
		showVersion   = flag.Bool("version", false, "Show version and exit.")
		systemBeat    = flag.Bool("beat.system", false, "Expose system stats.")
	)
	flag.Parse()

	if *showVersion {
		fmt.Print(version.Print(serviceName))
		os.Exit(0)
	}

	// Configure logging
	log.SetLevel(log.InfoLevel)
	log.SetFormatter(&log.JSONFormatter{
		FieldMap: log.FieldMap{
			log.FieldKeyMsg: "message",
		},
	})

	// Parse the comma-separated list of Beat URIs
	beatURLList := strings.Split(*beatURIs, ",")

	// Create a reusable HTTP client
	httpClient := &http.Client{Timeout: *beatTimeout}

	// Setup signal handling for graceful shutdown
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

	// Prometheus registry
	registry := prometheus.NewRegistry()
	registry.MustRegister(version.NewCollector(serviceName))

	// Discover Beat types
	for _, beatURI := range beatURLList {
		if err := discoverBeatType(httpClient, beatURI, registry, *systemBeat); err != nil {
			log.Warnf("Failed to discover beat type at %s: %v", beatURI, err)
		}
	}

	// Setup Prometheus metrics endpoint
	http.Handle(*metricsPath, promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		ErrorLog:           log.New(),
		DisableCompression: false,
		ErrorHandling:      promhttp.ContinueOnError,
	}))

	http.HandleFunc("/", indexHandler(*metricsPath))

	// Start the server
	go startHTTPServer(*listenAddress, *tlsCertFile, *tlsKeyFile)

	<-stopCh
	log.Info("Exporter stopped gracefully")
}

// discoverBeatType attempts to load Beat info from a given URI and registers the collector if successful.
func discoverBeatType(client *http.Client, beatURI string, registry *prometheus.Registry, systemBeat bool) error {
	beatURL, err := url.Parse(beatURI)
	if err != nil {
		return fmt.Errorf("failed to parse beat URI: %w", err)
	}

	// Adjust transport for Unix socket
	if beatURL.Scheme == "unix" {
		unixPath := beatURL.Path
		beatURL.Scheme = "http"
		beatURL.Host = "localhost"
		beatURL.Path = ""
		client.Transport = &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", unixPath)
			},
		}
	}

	log.Infof("Trying to discover beat type at %s", beatURI)
	beatInfo, err := loadBeatType(client, *beatURL)
	if err != nil {
		return err // If it fails, return the error
	}

	// Register the collector for the discovered Beat
	mainCollector := collector.NewMainCollector(client, beatURL, serviceName, beatInfo, systemBeat)
	registry.MustRegister(mainCollector)

	log.Infof("Beat type loaded successfully from %s", beatURI)
	return nil
}

// indexHandler returns an HTTP handler that serves the index page.
func indexHandler(metricsPath string) http.HandlerFunc {
	indexHTML := `
<html>
	<head>
		<title>Beat Exporter</title>
	</head>
	<body>
		<h1>Beat Exporter</h1>
		<p>
			<a href='%s'>Metrics</a>
		</p>
	</body>
</html>
`
	index := []byte(fmt.Sprintf(strings.TrimSpace(indexHTML), metricsPath))

	return func(w http.ResponseWriter, r *http.Request) {
		w.Write(index)
	}
}

// loadBeatType fetches the Beat info from the provided URL.
func loadBeatType(client *http.Client, url url.URL) (*collector.BeatInfo, error) {
	response, err := client.Get(url.String())
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 response: %d", response.StatusCode)
	}

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	var beatInfo collector.BeatInfo
	if err := json.Unmarshal(bodyBytes, &beatInfo); err != nil {
		return nil, err
	}

	log.Infof("Target beat loaded: %v", beatInfo)
	return &beatInfo, nil
}

// startHTTPServer starts the HTTP server for Prometheus metrics.
func startHTTPServer(listenAddress, tlsCertFile, tlsKeyFile string) {
	log.Infof("Starting exporter at %s", listenAddress)
	if tlsCertFile != "" && tlsKeyFile != "" {
		if err := http.ListenAndServeTLS(listenAddress, tlsCertFile, tlsKeyFile, nil); err != nil {
			log.Fatalf("TLS server error: %v", err)
		}
	} else {
		if err := http.ListenAndServe(listenAddress, nil); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	}
}
