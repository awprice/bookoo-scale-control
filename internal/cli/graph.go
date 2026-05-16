package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/NimbleMarkets/ntcharts/canvas"
	"github.com/NimbleMarkets/ntcharts/linechart"
	"github.com/awprice/bookoo-scale-control/pkg/bookoo"
	"github.com/charmbracelet/lipgloss"
)

var (
	graphAxisStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	graphLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	graphTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	weightLineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))  // green
	flowLineStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("14")) // bright cyan
)

const (
	chartWidth  = 80
	chartHeight = 12
)

// printShotGraph writes the shot summary to w.
func printShotGraph(w io.Writer, measurements []bookoo.Measurement) {
	if len(measurements) < 2 {
		return
	}
	fmt.Fprint(w, renderShotGraph(measurements))
}

// smoothFlowRates returns a centred moving-average of flow rates over a
// ±3 sample window (~600 ms at 10 Hz), smoothing noise without shifting peaks.
func smoothFlowRates(measurements []bookoo.Measurement) []float64 {
	const half = 3
	n := len(measurements)
	out := make([]float64, n)
	for i := range measurements {
		lo, hi := i-half, i+half
		if lo < 0 {
			lo = 0
		}
		if hi >= n {
			hi = n - 1
		}
		sum := 0.0
		for j := lo; j <= hi; j++ {
			sum += measurements[j].FlowRate
		}
		out[i] = sum / float64(hi-lo+1)
	}
	return out
}

// renderShotGraph returns the shot summary as a string.
func renderShotGraph(measurements []bookoo.Measurement) string {
	if len(measurements) < 2 {
		return ""
	}

	// Use the scale's elapsed timestamp as the X axis (seconds).
	// Fall back to point index if the timer never started.
	maxT := measurements[len(measurements)-1].Timestamp.Seconds()
	getX := func(i int) float64 { return measurements[i].Timestamp.Seconds() }
	if maxT == 0 {
		maxT = float64(len(measurements) - 1)
		getX = func(i int) float64 { return float64(i) }
	}
	if maxT < 1 {
		maxT = 1
	}

	smoothedFlow := smoothFlowRates(measurements)
	for i, v := range smoothedFlow {
		if v > -0.075 && v < 0.075 {
			smoothedFlow[i] = 0
		}
	}

	maxWeight := 1.0
	minFlow, maxFlow := 0.0, 0.0
	for i, m := range measurements {
		w := m.Weight
		if w < 0 {
			w = 0
		}
		if w > maxWeight {
			maxWeight = w
		}
		if smoothedFlow[i] < minFlow {
			minFlow = smoothedFlow[i]
		}
		if smoothedFlow[i] > maxFlow {
			maxFlow = smoothedFlow[i]
		}
	}
	if minFlow == maxFlow {
		maxFlow = minFlow + 1
	}

	wc := linechart.New(chartWidth, chartHeight,
		0, maxT, 0, maxWeight,
		linechart.WithXYSteps(6, 4),
		linechart.WithStyles(graphAxisStyle, graphLabelStyle, weightLineStyle),
	)
	wc.DrawXYAxisAndLabel()
	clampWeight := func(w float64) float64 {
		if w < 0 {
			return 0
		}
		return w
	}
	for i := 1; i < len(measurements); i++ {
		wc.DrawBrailleLineWithStyle(
			canvas.Float64Point{X: getX(i - 1), Y: clampWeight(measurements[i-1].Weight)},
			canvas.Float64Point{X: getX(i), Y: clampWeight(measurements[i].Weight)},
			weightLineStyle,
		)
	}

	fc := linechart.New(chartWidth, chartHeight,
		0, maxT, minFlow, maxFlow,
		linechart.WithXYSteps(6, 4),
		linechart.WithStyles(graphAxisStyle, graphLabelStyle, flowLineStyle),
	)
	fc.DrawXYAxisAndLabel()
	for i := 1; i < len(measurements); i++ {
		fc.DrawBrailleLineWithStyle(
			canvas.Float64Point{X: getX(i - 1), Y: smoothedFlow[i-1]},
			canvas.Float64Point{X: getX(i), Y: smoothedFlow[i]},
			flowLineStyle,
		)
	}

	divider := strings.Repeat("─", chartWidth+6)
	var sb strings.Builder
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, graphTitleStyle.Render("Shot Summary"))
	fmt.Fprintln(&sb, divider)
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, graphTitleStyle.Render("Weight (g)"))
	fmt.Fprintln(&sb, wc.View())
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, graphTitleStyle.Render("Flow Rate (g/s)"))
	fmt.Fprintln(&sb, fc.View())
	fmt.Fprintln(&sb)
	return sb.String()
}
