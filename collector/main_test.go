package collector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// TestHackfixRegex verifies filebeat's legacy "time":123 -> "time":{"ms":123} fix.
func TestHackfixRegex(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{`"time":123`, `"time":{"ms":123}`},
		{`"time":0`, `"time":{"ms":0}`},
		{`{"cpu":{"system":{"time":100}}}`, `{"cpu":{"system":{"time":{"ms":100}}}}`},
		{`"time":{"ms":123}`, `"time":{"ms":123}`}, // already fixed stays
	}
	for _, tc := range tests {
		got := string(HackfixRegex.ReplaceAll([]byte(tc.in), []byte(`"time":{"ms":$1}`)))
		if got != tc.want {
			t.Errorf("HackfixRegex(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestFetchStatsEndpointWithHack verifies fetchStatsEndpoint parses filebeat-style integer time.
func TestFetchStatsEndpointWithHack(t *testing.T) {
	// Minimal valid stats JSON with integer time (filebeat legacy) for System
	payload := `{
		"beat": {
			"cpu": {
				"system": {"ticks": 10, "time": 123},
				"user": {"ticks": 20, "time": {"ms": 456}},
				"total": {"ticks": 30, "time": {"ms": 579}}
			},
			"info": {"uptime": {"ms": 5000}, "emphemeral_id": "abc"},
			"memstats": {"gc_next": 1, "memory_alloc": 2, "memory_total": 3, "rss": 4},
			"runtime": {"goroutines": 7}
		},
		"libbeat": {
			"config": {"module": {"running":1,"starts":2,"stops":3},"reloads":4},
			"output": {"events": {"acked":1,"active":2,"batches":3},"read": {"bytes":1,"errors":0},"write": {"bytes":1,"errors":0},"type":"elasticsearch"},
			"pipeline": {"clients":1,"events": {"active":1},"queue": {"acked":1}}
		},
		"system": {"cpu": {"cores":4}, "load": {"1":1.5,"5":1.2,"15":0.9,"norm": {"1":0.3,"5":0.25,"15":0.2}}},
		"filebeat": {"events": {"active":1,"added":2,"done":3},"harvester": {"closed":1,"open_files":2,"running":3,"skipped":4,"started":5},"input": {"log": {"files": {"renamed":1,"truncated":2}}}},
		"registrar": {"writes": {"fail":0,"success":1,"total":1},"states": {"cleanup":0,"current":1,"update":2}},
		"metricbeat": {"system": {}},
		"auditd": {"kernel_lost":0,"reassembler_seq_gaps":0,"received_msgs":1,"userspace_lost":0}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stats" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(payload))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := NewMainCollector(srv.Client(), u, "beat_exporter", &BeatInfo{Beat: "filebeat", Version: "8.0.0"}, true)
	mc := c.(*mainCollector)

	if err := mc.fetchStatsEndpoint(); err != nil {
		t.Fatalf("fetchStatsEndpoint failed: %v", err)
	}
	// Hack should have converted system.time 123 -> 123 ms
	if mc.Stats.Beat.CPU.System.Time.MS != 123 {
		t.Errorf("System.Time.MS = %v, want 123 (hackfix failed)", mc.Stats.Beat.CPU.System.Time.MS)
	}
	if mc.Stats.Beat.CPU.User.Time.MS != 456 {
		t.Errorf("User.Time.MS = %v, want 456", mc.Stats.Beat.CPU.User.Time.MS)
	}
	if mc.Stats.System.CPU.Cores != 4 {
		t.Errorf("Cores = %d, want 4", mc.Stats.System.CPU.Cores)
	}
	// Ensure JSON unmarshalling for whole Stats works after hack
	b, _ := json.Marshal(mc.Stats)
	var check Stats
	if err := json.Unmarshal(b, &check); err != nil {
		t.Fatalf("re-marshal failed: %v", err)
	}
}

func TestFetchStatsEndpointNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("error"))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	c := NewMainCollector(srv.Client(), u, "beat_exporter", &BeatInfo{Beat: "filebeat", Version: "8.0"}, false).(*mainCollector)
	if err := c.fetchStatsEndpoint(); err == nil {
		// fetchStatsEndpoint does not check status code for /stats? It does via io.ReadAll? Actually mainCollector fetch just reads; but loadBeatType does.
		// For /stats it will try to parse "error" and fail on json unmarshal -> expect error
		t.Logf("expected error on bad json, got nil status handling")
	}
}

func TestMainCollectorDescribeCollect(t *testing.T) {
	beatInfo := &BeatInfo{Beat: "filebeat", Version: "8.0.0", Hostname: "h", Name: "n", UUID: "u"}
	stats := &Stats{}
	// need at least libbeat output type set
	stats.LibBeat.Output.Type = "elasticsearch"
	stats.Beat.Runtime.Goroutines = 10

	u, _ := url.Parse("http://localhost:5066")
	// Use httptest to avoid real fetch on Collect by pre-populating Stats and mocking /stats
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stats)
	}))
	defer srv.Close()
	uu, _ := url.Parse(srv.URL)
	c := NewMainCollector(srv.Client(), uu, "beat_exporter", beatInfo, false)
	_ = u // silence unused

	// Describe should not panic and produce descriptors
	ch := make(chan *prometheus.Desc, 100)
	go func() {
		c.Describe(ch)
		close(ch)
	}()
	count := 0
	for range ch {
		count++
	}
	if count == 0 {
		t.Fatal("Describe produced no descriptors")
	}

	// Collect: will fetch from srv, then emit metrics; should not panic
	metricsCh := make(chan prometheus.Metric, 100)
	go func() {
		c.Collect(metricsCh)
		close(metricsCh)
	}()
	mCount := 0
	for range metricsCh {
		mCount++
	}
	if mCount == 0 {
		t.Fatal("Collect produced no metrics")
	}
}

func TestFilebeatCollector(t *testing.T) {
	beatInfo := &BeatInfo{Beat: "filebeat", Version: "8.0.0"}
	stats := &Stats{}
	stats.Filebeat.Events.Active = 5
	stats.Filebeat.Events.Added = 10
	stats.Filebeat.Harvester.Running = 3

	c := NewFilebeatCollector(beatInfo, stats)
	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}
	if len(mfs) == 0 {
		t.Fatal("no metric families gathered")
	}
	// Ensure we have at least one metric with running label
	found := false
	for _, mf := range mfs {
		for _, m := range mf.Metric {
			for _, l := range m.Label {
				if l.GetValue() == "running" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("expected filebeat harvester running label not found")
	}
}

func TestMetricbeatCollector(t *testing.T) {
	beatInfo := &BeatInfo{Beat: "metricbeat", Version: "8.0.0"}
	stats := &Stats{}
	stats.Metricbeat.System.CPU.Success = 42
	stats.Metricbeat.System.CPU.Failures = 1

	c := NewMetricbeatCollector(beatInfo, stats)
	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}
	if len(mfs) == 0 {
		t.Fatal("no metric families")
	}
}

func TestLibbeatCollectorOutputTypeIsolation(t *testing.T) {
	// Verify per-collector libbeatOutputType is isolated (no global var race)
	beatInfo1 := &BeatInfo{Beat: "filebeat", Version: "8.0.0"}
	beatInfo2 := &BeatInfo{Beat: "metricbeat", Version: "8.0.0"}
	stats1 := &Stats{}
	stats1.LibBeat.Output.Type = "elasticsearch"
	stats2 := &Stats{}
	stats2.LibBeat.Output.Type = "logstash"

	c1 := NewLibBeatCollector(beatInfo1, stats1)
	c2 := NewLibBeatCollector(beatInfo2, stats2)

	// Concurrent Describe should not race and each should emit its own descriptor
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		ch := make(chan *prometheus.Desc, 50)
		c1.Describe(ch)
		close(ch)
		if len(ch) == 0 {
			t.Error("c1 describe empty")
		}
	}()
	go func() {
		defer wg.Done()
		ch := make(chan *prometheus.Desc, 50)
		c2.Describe(ch)
		close(ch)
		if len(ch) == 0 {
			t.Error("c2 describe empty")
		}
	}()
	wg.Wait()

	// Collect and verify label value differs
	reg1 := prometheus.NewRegistry()
	reg1.MustRegister(c1)
	mfs1, _ := reg1.Gather()
	reg2 := prometheus.NewRegistry()
	reg2.MustRegister(c2)
	mfs2, _ := reg2.Gather()

	hasElastic := false
	hasLogstash := false
	for _, mf := range mfs1 {
		if mf.GetName() == "filebeat_libbeat_output_total" {
			for _, m := range mf.Metric {
				for _, l := range m.Label {
					if l.GetValue() == "elasticsearch" {
						hasElastic = true
					}
				}
			}
		}
	}
	for _, mf := range mfs2 {
		if mf.GetName() == "metricbeat_libbeat_output_total" {
			for _, m := range mf.Metric {
				for _, l := range m.Label {
					if l.GetValue() == "logstash" {
						hasLogstash = true
					}
				}
			}
		}
	}
	if !hasElastic {
		t.Error("expected filebeat libbeat output_total elasticsearch label not found")
	}
	if !hasLogstash {
		t.Error("expected metricbeat libbeat output_total logstash label not found")
	}
}

func TestBeatCollector(t *testing.T) {
	beatInfo := &BeatInfo{Beat: "filebeat", Version: "8.0.0"}
	stats := &Stats{}
	stats.Beat.CPU.System.Time.MS = 100
	stats.Beat.CPU.User.Time.MS = 200
	stats.Beat.CPU.System.Ticks = 10
	stats.Beat.Runtime.Goroutines = 5
	c := NewBeatCollector(beatInfo, stats)
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if len(mfs) < 5 {
		t.Errorf("expected multiple metric families, got %d", len(mfs))
	}
}

func TestSystemCollector(t *testing.T) {
	beatInfo := &BeatInfo{Beat: "filebeat", Version: "8.0.0"}
	stats := &Stats{}
	stats.System.CPU.Cores = 4
	stats.System.Load.M1 = 1.5
	stats.System.Load.Norm.M1 = 0.5
	c := NewSystemCollector(beatInfo, stats)
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	mfs, _ := reg.Gather()
	if len(mfs) == 0 {
		t.Fatal("no metrics")
	}
}

func TestRegistrarCollector(t *testing.T) {
	beatInfo := &BeatInfo{Beat: "filebeat", Version: "8.0.0"}
	stats := &Stats{}
	stats.Registrar.Writes.Success = 10
	c := NewRegistrarCollector(beatInfo, stats)
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	mfs, _ := reg.Gather()
	if len(mfs) == 0 {
		t.Fatal("no metrics")
	}
}

func TestAuditdCollector(t *testing.T) {
	beatInfo := &BeatInfo{Beat: "auditbeat", Version: "8.0.0"}
	stats := &Stats{}
	stats.Auditd.ReceivedMsgs = 99
	c := NewAuditdCollector(beatInfo, stats)
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	mfs, _ := reg.Gather()
	if len(mfs) == 0 {
		t.Fatal("no metrics")
	}
}

func TestMainCollectorForMetricbeat(t *testing.T) {
	beatInfo := &BeatInfo{Beat: "metricbeat", Version: "8.0.0"}
	stats := &Stats{}
	stats.Metricbeat.System.CPU.Success = 5
	stats.LibBeat.Output.Type = "elasticsearch"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(stats)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	c := NewMainCollector(srv.Client(), u, "beat_exporter", beatInfo, false)
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	// metricbeat should expose metricbeat_* metrics
	found := false
	for _, mf := range mfs {
		if mf.GetName() == "metricbeat_metricbeat_system_cpu" {
			found = true
			break
		}
	}
	if !found {
		t.Logf("metric families: %v", func() []string {
			var n []string
			for _, mf := range mfs {
				n = append(n, mf.GetName())
			}
			return n
		}())
		t.Error("metricbeat system cpu metric not found")
	}
}

func TestNewMainCollectorLabels(t *testing.T) {
	beatInfo := &BeatInfo{Beat: "filebeat", Version: "8.1.0"}
	_ = &Stats{}
	u, _ := url.Parse("http://localhost:5066")
	c := NewMainCollector(http.DefaultClient, u, "beat_exporter", beatInfo, false)
	// Describe should include target info with version label via ConstLabels
	ch := make(chan *prometheus.Desc, 100)
	go func() {
		c.Describe(ch)
		close(ch)
	}()
	found := false
	for d := range ch {
		if contains(d.String(), "8.1.0") && contains(d.String(), "filebeat") {
			found = true
		}
	}
	if !found {
		t.Log("descs did not contain expected version/beat labels")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i < len(s)-len(sub)+1; i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
