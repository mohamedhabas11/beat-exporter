package collector

import (
	"log"

	"github.com/prometheus/client_golang/prometheus"
)

// Filebeat represents the JSON structure for Filebeat metrics.
type Filebeat struct {
	Events struct {
		Active float64 `json:"active"`
		Added  float64 `json:"added"`
		Done   float64 `json:"done"`
	} `json:"events"`

	Harvester struct {
		Closed    float64 `json:"closed"`
		OpenFiles float64 `json:"open_files"`
		Running   float64 `json:"running"`
		Skipped   float64 `json:"skipped"`
		Started   float64 `json:"started"`
	} `json:"harvester"`

	Input struct {
		Log struct {
			Files struct {
				Renamed   float64 `json:"renamed"`
				Truncated float64 `json:"truncated"`
			} `json:"files"`
		} `json:"log"`
	} `json:"input"`
}

// filebeatCollector holds Filebeat metrics and their descriptions.
type filebeatCollector struct {
	beatInfo *BeatInfo
	stats    *Stats
	metrics  map[string]*prometheus.Desc
}

// NewFilebeatCollector creates a new instance of the Filebeat collector.
func NewFilebeatCollector(beatInfo *BeatInfo, stats *Stats) prometheus.Collector {
	metrics := make(map[string]*prometheus.Desc)

	// Define all the metrics we want to track, stored in a map for easier reuse.
	metricLabels := []struct {
		name      string
		help      string
		labels    prometheus.Labels
		subsystem string
	}{
		{"events_active", "Number of active events", prometheus.Labels{"event": "active"}, "events"},
		{"events_added", "Number of added events", prometheus.Labels{"event": "added"}, "events"},
		{"events_done", "Number of completed events", prometheus.Labels{"event": "done"}, "events"},
		{"harvester_closed", "Number of closed harvesters", prometheus.Labels{"harvester": "closed"}, "harvester"},
		{"harvester_open_files", "Number of open files by harvesters", prometheus.Labels{"harvester": "open_files"}, "harvester"},
		{"harvester_running", "Number of running harvesters", prometheus.Labels{"harvester": "running"}, "harvester"},
		{"harvester_skipped", "Number of skipped harvesters", prometheus.Labels{"harvester": "skipped"}, "harvester"},
		{"harvester_started", "Number of started harvesters", prometheus.Labels{"harvester": "started"}, "harvester"},
		{"input_log_files_renamed", "Number of renamed log files", prometheus.Labels{"files": "renamed"}, "input_log"},
		{"input_log_files_truncated", "Number of truncated log files", prometheus.Labels{"files": "truncated"}, "input_log"},
	}

	for _, label := range metricLabels {
		metrics[label.name] = prometheus.NewDesc(
			prometheus.BuildFQName(beatInfo.Beat, label.subsystem, label.name),
			label.help,
			nil, label.labels,
		)
	}

	return &filebeatCollector{
		beatInfo: beatInfo,
		stats:    stats,
		metrics:  metrics,
	}
}

// Describe sends the metrics descriptions to the Prometheus channel.
func (c *filebeatCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range c.metrics {
		ch <- desc
	}
}

// Collect fetches the latest metrics and sends them to the Prometheus channel.
func (c *filebeatCollector) Collect(ch chan<- prometheus.Metric) {
	metricValues := map[string]float64{
		"events_active":             c.stats.Filebeat.Events.Active,
		"events_added":              c.stats.Filebeat.Events.Added,
		"events_done":               c.stats.Filebeat.Events.Done,
		"harvester_closed":          c.stats.Filebeat.Harvester.Closed,
		"harvester_open_files":      c.stats.Filebeat.Harvester.OpenFiles,
		"harvester_running":         c.stats.Filebeat.Harvester.Running,
		"harvester_skipped":         c.stats.Filebeat.Harvester.Skipped,
		"harvester_started":         c.stats.Filebeat.Harvester.Started,
		"input_log_files_renamed":   c.stats.Filebeat.Input.Log.Files.Renamed,
		"input_log_files_truncated": c.stats.Filebeat.Input.Log.Files.Truncated,
	}

	for key, val := range metricValues {
		if desc, ok := c.metrics[key]; ok {
			metric, err := prometheus.NewConstMetric(desc, prometheus.GaugeValue, val)
			if err != nil {
				log.Printf("error creating metric %s: %v", key, err)
				continue
			}
			ch <- metric
		}
	}
}
