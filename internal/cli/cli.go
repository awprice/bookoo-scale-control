package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/awprice/bookoo-scale-control/pkg/bookoo"
)

const usage = `Usage: bookoo <command> [flags]

Shot commands:
  monitor                        Stream live weight measurements until Ctrl+C
  tare                           Tare the scale (zero the weight)
  shot                           Tare, start the timer, and display real-time charts until Ctrl+C

Timer commands:
  start                          Start the built-in timer
  stop                           Stop the built-in timer
  reset                          Stop the timer and reset it to zero

Settings commands:
  settings                       Display current scale settings
  beep <0-5>                     Set speaker volume (0=silent, 5=loudest)
  auto-off <5-30>                Set inactivity auto-off timeout in minutes
  smoothing on|off               Enable or disable flow rate smoothing
  calibrate                      Run calibration routine (scale must be empty)
  stop-condition flow|container  Set auto-stop trigger (Themis Ultra only)

Flags:
  -timeout duration   How long to scan before giving up (default 30s)

Other:
  demo                           Simulate a shot with fake data (no scale required)
`

// Scale is the interface used by commands. It is satisfied by *bookoo.Scale.
type Scale interface {
	Tare() error
	StartTimer() error
	StopTimer() error
	ResetTimer() error
	TareAndStart() error
	SetBeepLevel(level int) error
	SetAutoOff(minutes int) error
	SetFlowSmoothing(enabled bool) error
	Calibrate() error
	SetStopCondition(cond bookoo.StopCondition) error
	Measurements() <-chan bookoo.Measurement
	Close() error
}

// App is the bookoo CLI application.
type App struct {
	connector func(ctx context.Context) (Scale, error)
}

// New returns an App wired to a real Bookoo scale.
func New() *App {
	return &App{
		connector: func(ctx context.Context) (Scale, error) {
			return bookoo.Connect(ctx)
		},
	}
}

// Run executes the CLI with the provided arguments (typically os.Args[1:]).
func (a *App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("no command specified")
	}

	cmd := args[0]

	fs := flag.NewFlagSet("bookoo", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	timeout := fs.Duration("timeout", 30*time.Second, "scan timeout")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	posArgs := fs.Args()

	scanCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	switch cmd {
	case "demo":
		return a.demo(ctx)
	case "settings":
		return a.settings(scanCtx)
	case "monitor":
		return a.monitor(ctx, scanCtx)
	case "tare":
		return a.command(scanCtx, "Tared.", func(s Scale) error { return s.Tare() })
	case "shot":
		return a.shot(ctx, scanCtx)
	case "start":
		return a.command(scanCtx, "Timer started.", func(s Scale) error { return s.StartTimer() })
	case "stop":
		return a.command(scanCtx, "Timer stopped.", func(s Scale) error { return s.StopTimer() })
	case "reset":
		return a.command(scanCtx, "Timer reset.", func(s Scale) error {
			if err := s.StopTimer(); err != nil {
				return err
			}
			return s.ResetTimer()
		})

	case "beep":
		level, err := parseIntArg(posArgs, "beep", 0, 5)
		if err != nil {
			return err
		}
		return a.command(scanCtx, fmt.Sprintf("Beep level set to %d.", level), func(s Scale) error {
			return s.SetBeepLevel(level)
		})

	case "auto-off":
		minutes, err := parseIntArg(posArgs, "auto-off", 5, 30)
		if err != nil {
			return err
		}
		return a.command(scanCtx, fmt.Sprintf("Auto-off set to %d minutes.", minutes), func(s Scale) error {
			return s.SetAutoOff(minutes)
		})

	case "smoothing":
		enabled, err := parseOnOff(posArgs, "smoothing")
		if err != nil {
			return err
		}
		state := map[bool]string{true: "enabled", false: "disabled"}[enabled]
		return a.command(scanCtx, fmt.Sprintf("Flow smoothing %s.", state), func(s Scale) error {
			return s.SetFlowSmoothing(enabled)
		})

	case "calibrate":
		return a.command(scanCtx, "Calibration started.", func(s Scale) error { return s.Calibrate() })

	case "stop-condition":
		cond, err := parseStopCondition(posArgs)
		if err != nil {
			return err
		}
		return a.command(scanCtx, "Stop condition updated.", func(s Scale) error {
			return s.SetStopCondition(cond)
		})

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n\n%s", cmd, usage)
		return fmt.Errorf("unknown command: %q", cmd)
	}
}

func (a *App) settings(scanCtx context.Context) error {
	scale, err := a.connect(scanCtx)
	if err != nil {
		return err
	}
	defer scale.Close()

	var m bookoo.Measurement
	select {
	case m, _ = <-scale.Measurements():
	case <-time.After(2 * time.Second):
		return fmt.Errorf("timed out waiting for settings")
	}

	smoothing := map[bool]string{true: "on", false: "off"}[m.FlowSmoothing]
	stopCond := map[bookoo.StopCondition]string{
		bookoo.StopConditionFlowStops:        "flow stops",
		bookoo.StopConditionContainerRemoved: "container removed",
	}[m.StopCondition]

	fmt.Printf("Current settings:\n")
	fmt.Printf("  Beep level:     %d\n", m.BeepLevel)
	fmt.Printf("  Auto-off:       %d min\n", m.AutoOff)
	fmt.Printf("  Flow smoothing: %s\n", smoothing)
	fmt.Printf("  Stop condition: %s\n", stopCond)
	return nil
}

func (a *App) monitor(ctx, scanCtx context.Context) error {
	scale, err := a.connect(scanCtx)
	if err != nil {
		return err
	}
	defer scale.Close()

	return stream(ctx, scale, "Connected. Press Ctrl+C to disconnect.", func() {
		fmt.Println("\nDisconnecting...")
	})
}

func (a *App) shot(ctx, scanCtx context.Context) error {
	scale, err := a.connect(scanCtx)
	if err != nil {
		return err
	}
	defer scale.Close()

	if err := scale.TareAndStart(); err != nil {
		return fmt.Errorf("shot: %w", err)
	}

	if isTerminal() {
		return runShotTUI(ctx, scale)
	}
	return runShotStream(ctx, scale)
}

// runShotStream is the non-interactive fallback used when stdout is not a TTY.
func runShotStream(ctx context.Context, scale Scale) error {
	fmt.Println("Shot started. Press Ctrl+C to stop.")
	fmt.Println()

	var measurements []bookoo.Measurement
	done := make(chan struct{})
	go func() {
		defer close(done)
		started := false
		for m := range scale.Measurements() {
			if !started && m.Timestamp > 0 && m.Timestamp < 1*time.Second {
				started = true
			}
			if started {
				measurements = append(measurements, m)
			}
			fmt.Printf("\rWeight: %7.2f g  Flow: %+6.2f g/s  Battery: %3d%%  Time: %s    ",
				m.Weight, m.FlowRate, m.Battery, formatDuration(m.Timestamp))
		}
		fmt.Println()
	}()

	select {
	case <-ctx.Done():
		fmt.Println("\nStopping timer...")
		scale.StopTimer()
		scale.Close()
	case <-done:
		fmt.Println("Scale disconnected.")
	}

	<-done
	printShotGraph(os.Stdout, measurements)
	return nil
}

func (a *App) demo(ctx context.Context) error {
	fmt.Println("Scanning for Bookoo scale...")
	select {
	case <-time.After(1500 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	fmt.Println("Connected to Bookoo Themis Ultra.")

	scale := newDemoScale()
	if err := scale.TareAndStart(); err != nil {
		return err
	}
	return runShotTUI(ctx, scale)
}

// command connects, runs fn, waits for one measurement to confirm the write
// was transmitted, then disconnects.
func (a *App) command(scanCtx context.Context, msg string, fn func(Scale) error) error {
	scale, err := a.connect(scanCtx)
	if err != nil {
		return err
	}
	defer scale.Close()

	if err := fn(scale); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}

	select {
	case <-scale.Measurements():
	case <-time.After(2 * time.Second):
	}

	fmt.Println(msg)
	return nil
}

func (a *App) connect(ctx context.Context) (Scale, error) {
	fmt.Println("Scanning for Bookoo scale...")
	return a.connector(ctx)
}

// stream prints live measurements until ctx is cancelled or the scale disconnects.
// onCancel is called when ctx is cancelled (e.g. Ctrl+C).
func stream(ctx context.Context, scale Scale, readyMsg string, onCancel func()) error {
	fmt.Println(readyMsg)
	fmt.Println()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for m := range scale.Measurements() {
			fmt.Printf("\rWeight: %7.2f g  Flow: %+6.2f g/s  Battery: %3d%%  Time: %s    ",
				m.Weight, m.FlowRate, m.Battery, formatDuration(m.Timestamp))
		}
		fmt.Println()
	}()

	select {
	case <-ctx.Done():
		onCancel()
	case <-done:
		fmt.Println("Scale disconnected.")
	}
	return nil
}

func formatDuration(d time.Duration) string {
	d = d.Round(100 * time.Millisecond)
	m := d / time.Minute
	s := (d % time.Minute) / time.Second
	ds := (d % time.Second) / (100 * time.Millisecond)
	return fmt.Sprintf("%02d:%02d.%d", m, s, ds)
}

// parseIntArg parses a single positional integer argument within [min, max].
func parseIntArg(args []string, cmd string, min, max int) (int, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("usage: bookoo %s <%d-%d>", cmd, min, max)
	}
	n, err := strconv.Atoi(args[0])
	if err != nil || n < min || n > max {
		return 0, fmt.Errorf("%s: value must be %d–%d, got %q", cmd, min, max, args[0])
	}
	return n, nil
}

// parseOnOff parses a single "on" or "off" positional argument.
func parseOnOff(args []string, cmd string) (bool, error) {
	if len(args) != 1 {
		return false, fmt.Errorf("usage: bookoo %s on|off", cmd)
	}
	switch strings.ToLower(args[0]) {
	case "on":
		return true, nil
	case "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s: expected on or off, got %q", cmd, args[0])
	}
}

// parseStopCondition parses "flow" or "container" into a StopCondition.
func parseStopCondition(args []string) (bookoo.StopCondition, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("usage: bookoo stop-condition flow|container")
	}
	switch strings.ToLower(args[0]) {
	case "flow":
		return bookoo.StopConditionFlowStops, nil
	case "container":
		return bookoo.StopConditionContainerRemoved, nil
	default:
		return 0, fmt.Errorf("stop-condition: expected flow or container, got %q", args[0])
	}
}
