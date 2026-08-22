package monitoring

import (
	"strings"
	"testing"
	"time"
)

func TestSummaryQueriesSelectNodeThroughUnameInfo(t *testing.T) {
	for name, query := range summaryQueries(`node"one`) {
		if !strings.Contains(query, `and on(instance) node_uname_info{nodename="node\"one"}`) {
			t.Errorf("%s query does not select the node through node_uname_info: %s", name, query)
		}
	}
}

func TestChartRange(t *testing.T) {
	tests := map[string]struct {
		duration time.Duration
		step     time.Duration
	}{
		"10m": {10 * time.Minute, 10 * time.Second},
		"1h":  {time.Hour, 30 * time.Second},
		"12h": {12 * time.Hour, 2 * time.Minute},
		"24h": {24 * time.Hour, 5 * time.Minute},
		"7d":  {7 * 24 * time.Hour, 30 * time.Minute},
		"30d": {30 * 24 * time.Hour, 2 * time.Hour},
	}

	for value, want := range tests {
		duration, step, err := chartRange(value)
		if err != nil || duration != want.duration || step != want.step {
			t.Errorf("chartRange(%q) = (%s, %s, %v), want (%s, %s, nil)", value, duration, step, err, want.duration, want.step)
		}
	}

	if _, _, err := chartRange("6h"); err == nil {
		t.Fatal("chartRange(6h) should reject an unsupported range")
	}
}
