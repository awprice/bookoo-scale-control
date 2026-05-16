package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/awprice/bookoo-scale-control/pkg/bookoo"
	tea "github.com/charmbracelet/bubbletea"
)


// measurementMsg carries a single scale reading into the bubbletea update loop.
type measurementMsg bookoo.Measurement

// shotDoneMsg signals that the measurements channel has closed.
type shotDoneMsg struct{}

type shotModel struct {
	ctx          context.Context
	scale        Scale
	measurements []bookoo.Measurement
}

func (m shotModel) Init() tea.Cmd {
	return waitForMeasurement(m.ctx, m.scale.Measurements())
}

func (m shotModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case measurementMsg:
		m.measurements = append(m.measurements, bookoo.Measurement(msg))
		return m, waitForMeasurement(m.ctx, m.scale.Measurements())
	case shotDoneMsg:
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.scale.StopTimer()
			m.scale.Close() // closes the channel; waitForMeasurement returns shotDoneMsg
		}
	}
	return m, nil
}

func (m shotModel) View() string {
	var sb strings.Builder

	if len(m.measurements) == 0 {
		sb.WriteString("Shot started. Waiting for first measurement...\n")
		return sb.String()
	}

	l := m.measurements[len(m.measurements)-1]
	fmt.Fprintf(&sb, "Weight: %7.2f g  Flow: %+6.2f g/s  Battery: %3d%%  Time: %s\n",
		l.Weight, l.FlowRate, l.Battery, formatDuration(l.Timestamp))

	if len(m.measurements) >= 2 {
		sb.WriteString(renderShotGraph(m.measurements))
	}

	sb.WriteString("Press Ctrl+C to stop")
	return sb.String()
}

// waitForMeasurement blocks until the next measurement arrives or ctx is done.
func waitForMeasurement(ctx context.Context, ch <-chan bookoo.Measurement) tea.Cmd {
	return func() tea.Msg {
		select {
		case m, ok := <-ch:
			if !ok {
				return shotDoneMsg{}
			}
			return measurementMsg(m)
		case <-ctx.Done():
			return shotDoneMsg{}
		}
	}
}

// runShotTUI runs the real-time shot display using bubbletea.
// The last rendered frame stays on screen as the final summary.
func runShotTUI(ctx context.Context, scale Scale) error {
	m := shotModel{ctx: ctx, scale: scale}
	_, err := tea.NewProgram(m).Run()
	return err
}

// isTerminal reports whether os.Stdout is an interactive terminal.
func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
