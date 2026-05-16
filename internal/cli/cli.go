package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/awprice/bookoo-scale-control/pkg/bookoo"
)

const usage = `Usage: bookoo <command> [flags]

Commands:
  monitor    Stream live weight measurements until Ctrl+C
  tare       Tare the scale (zero the weight)
  shot       Tare, start the timer, and stream live measurements until Ctrl+C
  start      Start the built-in timer
  stop       Stop the built-in timer
  reset      Stop the timer and reset it to zero

Flags:
  -timeout duration   How long to scan before giving up (default 30s)
`

// Scale is the interface used by commands. It is satisfied by *bookoo.Scale.
type Scale interface {
	Tare() error
	StartTimer() error
	StopTimer() error
	ResetTimer() error
	TareAndStart() error
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

	scanCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	switch cmd {
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
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n\n%s", cmd, usage)
		return fmt.Errorf("unknown command: %q", cmd)
	}
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

	return stream(ctx, scale, "Shot started. Press Ctrl+C to stop.", func() {
		fmt.Println("\nStopping timer...")
		scale.StopTimer()
	})
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
