package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
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

func main() {
	log.SetFlags(0)

	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	timeout := fs.Duration("timeout", 30*time.Second, "scan timeout")
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	cmd := os.Args[1]
	if err := fs.Parse(os.Args[2:]); err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	scanCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	switch cmd {
	case "monitor":
		runMonitor(ctx, scanCtx)
	case "tare":
		runCmd(scanCtx, "Tared.", func(s *bookoo.Scale) error { return s.Tare() })
	case "shot":
		runShot(ctx, scanCtx)
	case "start":
		runCmd(scanCtx, "Timer started.", func(s *bookoo.Scale) error { return s.StartTimer() })
	case "stop":
		runCmd(scanCtx, "Timer stopped.", func(s *bookoo.Scale) error { return s.StopTimer() })
	case "reset":
		runCmd(scanCtx, "Timer reset.", func(s *bookoo.Scale) error {
			if err := s.StopTimer(); err != nil {
				return err
			}
			return s.ResetTimer()
		})
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n\n%s", cmd, usage)
		os.Exit(1)
	}
}

// runMonitor connects and streams live measurements until the context is cancelled.
func runMonitor(ctx, scanCtx context.Context) {
	scale := mustConnect(scanCtx)
	defer scale.Close()

	stream(ctx, scale, "Connected. Press Ctrl+C to disconnect.", func() {
		fmt.Println("\nDisconnecting...")
	})
}

// runShot tares the scale, starts the timer, streams live measurements, then
// stops the timer when the user presses Ctrl+C.
func runShot(ctx, scanCtx context.Context) {
	scale := mustConnect(scanCtx)
	defer scale.Close()

	if err := scale.TareAndStart(); err != nil {
		log.Fatalf("shot: %v", err)
	}

	stream(ctx, scale, "Shot started. Press Ctrl+C to stop.", func() {
		fmt.Println("\nStopping timer...")
		scale.StopTimer()
	})
}

// mustConnect scans for a Bookoo scale and connects, or exits on failure.
func mustConnect(scanCtx context.Context) *bookoo.Scale {
	fmt.Println("Scanning for Bookoo scale...")
	scale, err := bookoo.Connect(scanCtx)
	if err != nil {
		log.Fatal(err)
	}
	return scale
}

// stream prints live measurements until ctx is cancelled or the scale disconnects.
// readyMsg is printed once the stream begins. onCancel is called when ctx is cancelled.
func stream(ctx context.Context, scale *bookoo.Scale, readyMsg string, onCancel func()) {
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
}

// runCmd connects, executes fn, prints msg, then disconnects.
// It waits for one incoming measurement before closing so that the BLE write
// is fully transmitted before the connection is torn down.
func runCmd(scanCtx context.Context, msg string, fn func(*bookoo.Scale) error) {
	fmt.Println("Scanning for Bookoo scale...")

	scale, err := bookoo.Connect(scanCtx)
	if err != nil {
		log.Fatal(err)
	}
	defer scale.Close()

	if err := fn(scale); err != nil {
		log.Fatalf("command failed: %v", err)
	}

	// Wait for one notification to confirm the scale has processed the command
	// before we disconnect.
	select {
	case <-scale.Measurements():
	case <-time.After(2 * time.Second):
	}

	fmt.Println(msg)
}

// formatDuration formats a duration as mm:ss.t for display.
func formatDuration(d time.Duration) string {
	d = d.Round(100 * time.Millisecond)
	m := d / time.Minute
	s := (d % time.Minute) / time.Second
	ds := (d % time.Second) / (100 * time.Millisecond)
	return fmt.Sprintf("%02d:%02d.%d", m, s, ds)
}
