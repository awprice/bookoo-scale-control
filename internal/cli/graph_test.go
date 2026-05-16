package cli

import (
	"bytes"
	"os"
	"regexp"
	"testing"

	"github.com/awprice/bookoo-scale-control/pkg/bookoo"
)

func TestPrintShotGraph_tooFewPoints(t *testing.T) {
	var buf bytes.Buffer
	printShotGraph(&buf, nil)
	if buf.Len() != 0 {
		t.Error("expected no output for nil measurements")
	}
	printShotGraph(&buf, []bookoo.Measurement{{Weight: 1.0}})
	if buf.Len() != 0 {
		t.Error("expected no output for a single measurement")
	}
}

func TestPrintShotGraph_producesOutput(t *testing.T) {
	var buf bytes.Buffer
	printShotGraph(&buf, fakeShotMeasurements())
	if buf.Len() == 0 {
		t.Error("expected graph output, got none")
	}
}

func TestPrintShotGraph_noTimestamps(t *testing.T) {
	// All timestamps zero — should fall back to using point index as X axis.
	measurements := []bookoo.Measurement{
		{Weight: 0, FlowRate: 0},
		{Weight: 10, FlowRate: 1.8},
		{Weight: 20, FlowRate: 2.0},
		{Weight: 30, FlowRate: 1.5},
	}
	var buf bytes.Buffer
	printShotGraph(&buf, measurements)
	if buf.Len() == 0 {
		t.Error("expected graph output even with zero timestamps")
	}
}

// TestGenerateFakeGraph writes a colour-stripped version of the shot graph to
// /tmp/bookoo-fake-graph.txt for embedding in the README.
// Run with: GENERATE_FAKE_GRAPH=1 go test -run TestGenerateFakeGraph ./internal/cli/
func TestGenerateFakeGraph(t *testing.T) {
	if os.Getenv("GENERATE_FAKE_GRAPH") == "" {
		t.Skip("set GENERATE_FAKE_GRAPH=1 to generate the README graph")
	}
	var buf bytes.Buffer
	printShotGraph(&buf, fakeShotMeasurements())
	clean := stripANSI(buf.String())
	if err := os.WriteFile("/tmp/bookoo-fake-graph.txt", []byte(clean), 0644); err != nil {
		t.Fatal(err)
	}
	t.Log("graph written to /tmp/bookoo-fake-graph.txt")
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[mGKHF]`)

func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

