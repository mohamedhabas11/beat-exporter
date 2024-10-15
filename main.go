package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	"github.com/trustpilot/beat-exporter/collector"
	"github.com/trustpilot/beat-exporter/internal/service"
)

const (
	serviceName = "beat_exporter"
)

func main() {
	var (
		Name          = serviceName
		listenAddress = flag.String("web.listen-address", ":9479", "Address to listen on for web interface and telemetry.")
		tlsCertFile   = flag.String("tls.certfile", "", "TLS certs file if you want to use tls instead of http")
		tlsKeyFile    = flag.String("tls.keyfile", "", "TLS key file if you want to use tls instead of http")
		metricsPath   = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
		beatURIs      = flag.String("beat.uris", "http://localhost:5066", "Comma-separated list of HTTP API addresses of Beats.")
		beatTimeout   = flag.Duration("beat.timeout", 10*time.Second, "Timeout for trying to get stats from Beats.")
		showVersion   = flag.Bool("version", false, "Show version and exit")
		systemBeat    = flag.Bool("beat.system", false, "Expose system stats")
	)
	flag.Parse()

	if *showVersion {
		fmt.Print(version.Print(Name))
		os.Exit(0)
	}

	log.SetLevel(log.DebugLevel)

	log.SetFormatter(&log.JSONFormatter{
		FieldMap: log.FieldMap{
			log.FieldKeyMsg: "message",
		},
	})

	// Parse the comma-separated list of Beat URIs
	beatURLList := strings.Split(*beatURIs, ",")

	// Create a single httpClient to be reused
	httpClient := &http.Client{
		Timeout: *beatTimeout,
	}

	// Setup HTTP handling
	stopCh := make(chan bool)
	err := service.SetupServiceListener(stopCh, serviceName, log.StandardLogger())
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Errorf("could not setup service listener: %v", err)
	}

	t := time.NewTicker(1 * time.Second)

	// version metric
	registry := prometheus.NewRegistry()
	versionMetric := version.NewCollector(Name)
	registry.MustRegister(versionMetric)

	for _, beatURI := range beatURLList {
		beatURL, err := url.Parse(beatURI)
		if err != nil {
			log.Fatalf("failed to parse beat.uri: %v, error: %v", beatURI, err)
		}

		// If Unix socket scheme is detected, adjust transport
		if beatURL.Scheme == "unix" {
			unixPath := beatURL.Path
			beatURL.Scheme = "http"
			beatURL.Host = "localhost"
			beatURL.Path = ""
			httpClient.Transport = &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", unixPath)
				},
			}
		}

		log.Infof("Exploring target for beat type at %v", beatURI)

		var beatInfo *collector.BeatInfo

		// Loop to discover the beat type, handle errors
	beatdiscovery:
		for {
			select {
			case <-t.C:
				beatInfo, err = loadBeatType(httpClient, *beatURL)
				if err != nil {
					log.Errorf("Could not load beat type for %v, retrying in 1s: %v", beatURI, err)
					continue
				}
				break beatdiscovery

			case <-stopCh:
				os.Exit(0) // signal received, stop gracefully
			}
		}

		// Register the collector for each discovered Beat
		mainCollector := collector.NewMainCollector(httpClient, beatURL, Name, beatInfo, *systemBeat)
		registry.MustRegister(mainCollector)
	}

	t.Stop() // Stop the ticker once beat discovery is done

	http.Handle(*metricsPath, promhttp.HandlerFor(
		registry,
		promhttp.HandlerOpts{
			ErrorLog:           log.New(),
			DisableCompression: false,
			ErrorHandling:      promhttp.ContinueOnError,
		}),
	)

	http.HandleFunc("/", IndexHandler(*metricsPath))

	log.WithFields(log.Fields{
		"addr": *listenAddress,
	}).Infof("Starting exporter with Beat URIs: %v", *beatURIs)

	go func() {
		defer func() {
			stopCh <- true
		}()

		log.Info("Starting listener")
		if *tlsCertFile != "" && *tlsKeyFile != "" {
			if err := http.ListenAndServeTLS(*listenAddress, *tlsCertFile, *tlsKeyFile, nil); err != nil {
				log.WithFields(log.Fields{
					"err": err,
				}).Errorf("TLS server quit with error: %v", err)
			}
		} else {
			if err := http.ListenAndServe(*listenAddress, nil); err != nil {
				log.WithFields(log.Fields{
					"err": err,
				}).Errorf("HTTP server quit with error: %v", err)
			}
		}
		log.Info("Listener exited")
	}()

	for {
		if <-stopCh {
			log.Info("Shutting down beats exporter")
			break
		}
	}
}

// IndexHandler returns a http handler with the correct metricsPath
func IndexHandler(metricsPath string) http.HandlerFunc {

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

func loadBeatType(client *http.Client, url url.URL) (*collector.BeatInfo, error) {
	beatInfo := &collector.BeatInfo{}

	response, err := client.Get(url.String())
	if err != nil {
		return beatInfo, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		log.Errorf("Beat URL: %q returned status code: %d", url.String(), response.StatusCode)
		return beatInfo, fmt.Errorf("received non-200 response: %d", response.StatusCode)
	}

	bodyBytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Error("Can't read body of response")
		return beatInfo, err
	}

	err = json.Unmarshal(bodyBytes, &beatInfo)
	if err != nil {
		log.Error("Could not parse JSON response for target")
		return beatInfo, err
	}

	log.WithFields(
		log.Fields{
			"beat":     beatInfo.Beat,
			"version":  beatInfo.Version,
			"name":     beatInfo.Name,
			"hostname": beatInfo.Hostname,
			"uuid":     beatInfo.UUID,
		}).Info("Target beat configuration loaded successfully!")

	return beatInfo, nil
}
