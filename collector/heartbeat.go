package collector

import (
	"github.com/prometheus/client_golang/prometheus"
)

type heartbeatCollector struct {
	beatInfo *BeatInfo
	stats    *Stats
	metrics  exportedMetrics
}

// NewHeartbeatCollector creates a stub collector for heartbeat beats.
// It exposes placeholder metrics; expand as heartbeat /stats schema is documented.
func NewHeartbeatCollector(beatInfo *BeatInfo, stats *Stats) prometheus.Collector {
	return &heartbeatCollector{
		beatInfo: beatInfo,
		stats:    stats,
		metrics: exportedMetrics{
			{
				desc: prometheus.NewDesc(
					prometheus.BuildFQName(beatInfo.Beat, "heartbeat", "monitors_active"),
					"heartbeat monitors active",
					nil, nil,
				),
				eval:    func(stats *Stats) float64 { return stats.Heartbeat.Monitors.Active },
				valType: prometheus.GaugeValue,
			},
			{
				desc: prometheus.NewDesc(
					prometheus.BuildFQName(beatInfo.Beat, "heartbeat", "monitors_total"),
					"heartbeat monitors total",
					nil, nil,
				),
				eval:    func(stats *Stats) float64 { return stats.Heartbeat.Monitors.Total },
				valType: prometheus.GaugeValue,
			},
		},
	}
}

// Describe returns all descriptions of the collector.
func (c *heartbeatCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range c.metrics {
		ch <- metric.desc
	}
}

// Collect returns the current state of all metrics of the collector.
func (c *heartbeatCollector) Collect(ch chan<- prometheus.Metric) {
	for _, i := range c.metrics {
		ch <- prometheus.MustNewConstMetric(i.desc, i.valType, i.eval(c.stats))
	}
}
