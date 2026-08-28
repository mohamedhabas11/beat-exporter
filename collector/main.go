package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"

	"github.com/prometheus/client_golang/prometheus"
	"log/slog"
)

type mainCollector struct {
	Collectors map[string]prometheus.Collector
	Stats      *Stats
	client     *http.Client
	beatURL    *url.URL
	name       string
	beatInfo   *BeatInfo
	targetDesc *prometheus.Desc
	targetUp   *prometheus.Desc
	metrics    exportedMetrics
	systemBeat bool
}

// HackfixRegex regex to replace JSON part
var HackfixRegex = regexp.MustCompile(`"time"\s*:\s*(\d+)`) // replaces time:123 to time.ms:123, only filebeat has different naming of time metric; whitespace-tolerant

// NewMainCollector constructor
func NewMainCollector(client *http.Client, url *url.URL, name string, beatInfo *BeatInfo, systemBeat bool) prometheus.Collector {
	instance := fmt.Sprintf("%s:%s", url.Hostname(), url.Port())
	beat := &mainCollector{
		Collectors: make(map[string]prometheus.Collector),
		Stats:      &Stats{},
		client:     client,
		beatURL:    url,
		name:       name,
		targetDesc: prometheus.NewDesc(
			prometheus.BuildFQName(name, "target", "info"),
			"target information",
			nil,
			prometheus.Labels{"version": beatInfo.Version, "beat": beatInfo.Beat, "uri": instance}),
		targetUp: prometheus.NewDesc(
			prometheus.BuildFQName("", beatInfo.Beat, "up"),
			"Target up",
			nil,
			nil),

		beatInfo:   beatInfo,
		metrics:    exportedMetrics{},
		systemBeat: systemBeat,
	}

	// Add specific collectors based on the beat type
	beat.Collectors["system"] = NewSystemCollector(beatInfo, beat.Stats)
	beat.Collectors["beat"] = NewBeatCollector(beatInfo, beat.Stats)
	beat.Collectors["libbeat"] = NewLibBeatCollector(beatInfo, beat.Stats)
	beat.Collectors["registrar"] = NewRegistrarCollector(beatInfo, beat.Stats)
	beat.Collectors["filebeat"] = NewFilebeatCollector(beatInfo, beat.Stats)
	beat.Collectors["metricbeat"] = NewMetricbeatCollector(beatInfo, beat.Stats)
	beat.Collectors["auditd"] = NewAuditdCollector(beatInfo, beat.Stats)
	beat.Collectors["heartbeat"] = NewHeartbeatCollector(beatInfo, beat.Stats)
	beat.Collectors["winlogbeat"] = NewWinlogbeatCollector(beatInfo, beat.Stats)

	return beat
}

// Describe returns all descriptions of the collector.
func (b *mainCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- b.targetDesc
	ch <- b.targetUp

	for _, metric := range b.metrics {
		ch <- metric.desc
	}

	// Describe the standard collectors
	if b.systemBeat {
		b.Collectors["system"].Describe(ch)
	}
	b.Collectors["beat"].Describe(ch)
	b.Collectors["libbeat"].Describe(ch)
	b.Collectors["auditd"].Describe(ch)

	// Handle custom collectors based on beat type
	switch b.beatInfo.Beat {
	case "filebeat":
		b.Collectors["filebeat"].Describe(ch)
		b.Collectors["registrar"].Describe(ch)
	case "metricbeat":
		b.Collectors["metricbeat"].Describe(ch)
	case "heartbeat":
		b.Collectors["heartbeat"].Describe(ch)
	case "winlogbeat":
		b.Collectors["winlogbeat"].Describe(ch)
	}
}

// Collect returns the current state of all metrics of the collector.
func (b *mainCollector) Collect(ch chan<- prometheus.Metric) {
	err := b.fetchStatsEndpoint()
	if err != nil {
		ch <- prometheus.MustNewConstMetric(b.targetUp, prometheus.GaugeValue, float64(0)) // Set target down
		slog.Error("Failed getting /stats endpoint of target", "error", err)
		return
	}

	ch <- prometheus.MustNewConstMetric(b.targetDesc, prometheus.GaugeValue, float64(1))
	ch <- prometheus.MustNewConstMetric(b.targetUp, prometheus.GaugeValue, float64(1)) // Set target up

	for _, i := range b.metrics {
		ch <- prometheus.MustNewConstMetric(i.desc, i.valType, i.eval(b.Stats))
	}

	// Collect metrics from standard collectors
	if b.systemBeat {
		b.Collectors["system"].Collect(ch)
	}
	b.Collectors["beat"].Collect(ch)
	b.Collectors["libbeat"].Collect(ch)
	b.Collectors["auditd"].Collect(ch)

	// Handle custom collectors per beat type
	switch b.beatInfo.Beat {
	case "filebeat":
		b.Collectors["filebeat"].Collect(ch)
		b.Collectors["registrar"].Collect(ch)
	case "metricbeat":
		b.Collectors["metricbeat"].Collect(ch)
	case "heartbeat":
		b.Collectors["heartbeat"].Collect(ch)
	case "winlogbeat":
		b.Collectors["winlogbeat"].Collect(ch)
	}
}

// fetchStatsEndpoint fetches the stats endpoint for the Beat.
func (b *mainCollector) fetchStatsEndpoint() error {
	response, err := b.client.Get(b.beatURL.String() + "/stats")
	if err != nil {
		slog.Error("Could not fetch stats endpoint of target", "url", b.beatURL.String())
		return err
	}
	defer response.Body.Close()

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		slog.Error("Can't read body of response")
		return err
	}

	// Apply a regex fix specifically for Filebeat
	bodyBytes = HackfixRegex.ReplaceAll(bodyBytes, []byte("\"time\":{\"ms\":$1}"))

	err = json.Unmarshal(bodyBytes, &b.Stats)
	if err != nil {
		slog.Error("Could not parse JSON response for target")
		return err
	}

	return nil
}
