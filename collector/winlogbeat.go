package collector

import (
	"github.com/prometheus/client_golang/prometheus"
)

type winlogbeatCollector struct {
	beatInfo *BeatInfo
	stats    *Stats
	metrics  exportedMetrics
}

// NewWinlogbeatCollector creates a stub collector for winlogbeat beats.
func NewWinlogbeatCollector(beatInfo *BeatInfo, stats *Stats) prometheus.Collector {
	return &winlogbeatCollector{
		beatInfo: beatInfo,
		stats:    stats,
		metrics: exportedMetrics{
			{
				desc: prometheus.NewDesc(
					prometheus.BuildFQName(beatInfo.Beat, "winlogbeat", "events_active"),
					"winlogbeat events active",
					nil, nil,
				),
				eval:    func(stats *Stats) float64 { return stats.Winlogbeat.Events.Active },
				valType: prometheus.GaugeValue,
			},
			{
				desc: prometheus.NewDesc(
					prometheus.BuildFQName(beatInfo.Beat, "winlogbeat", "events_total"),
					"winlogbeat events total",
					nil, nil,
				),
				eval:    func(stats *Stats) float64 { return stats.Winlogbeat.Events.Total },
				valType: prometheus.GaugeValue,
			},
		},
	}
}

// Describe returns all descriptions of the collector.
func (c *winlogbeatCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range c.metrics {
		ch <- metric.desc
	}
}

// Collect returns the current state of all metrics of the collector.
func (c *winlogbeatCollector) Collect(ch chan<- prometheus.Metric) {
	for _, i := range c.metrics {
		ch <- prometheus.MustNewConstMetric(i.desc, i.valType, i.eval(c.stats))
	}
}
