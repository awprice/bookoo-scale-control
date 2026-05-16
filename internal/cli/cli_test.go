package cli

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/awprice/bookoo-scale-control/pkg/bookoo"
)

// mockScale records which methods were called and returns configured errors.
type mockScale struct {
	mu           sync.Mutex
	calls        []string
	measurements chan bookoo.Measurement
	errs         map[string]error
	closed       bool
}

func newMockScale() *mockScale {
	return &mockScale{
		measurements: make(chan bookoo.Measurement, 16),
		errs:         map[string]error{},
	}
}

func (m *mockScale) record(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, name)
	return m.errs[name]
}

func (m *mockScale) Tare() error         { return m.record("Tare") }
func (m *mockScale) StartTimer() error   { return m.record("StartTimer") }
func (m *mockScale) StopTimer() error    { return m.record("StopTimer") }
func (m *mockScale) ResetTimer() error   { return m.record("ResetTimer") }
func (m *mockScale) TareAndStart() error { return m.record("TareAndStart") }

func (m *mockScale) Measurements() <-chan bookoo.Measurement { return m.measurements }

func (m *mockScale) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Close")
	if !m.closed {
		m.closed = true
		close(m.measurements)
	}
	return nil
}

// disconnect simulates the scale dropping the connection by closing the
// measurements channel, as would happen if the scale powered off.
func (m *mockScale) disconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.closed = true
		close(m.measurements)
	}
}

func (m *mockScale) called() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.calls))
	copy(out, m.calls)
	return out
}

// newTestApp returns an App that always connects to the given mock scale.
func newTestApp(scale *mockScale) *App {
	return &App{connector: func(ctx context.Context) (Scale, error) {
		return scale, nil
	}}
}

// sendMeasurement unblocks a command() call that waits for one notification.
func sendMeasurement(m *mockScale) {
	m.measurements <- bookoo.Measurement{}
}

// helpers

func assertCalled(t *testing.T, calls []string, method string) {
	t.Helper()
	for _, c := range calls {
		if c == method {
			return
		}
	}
	t.Errorf("expected %q to be called; got %v", method, calls)
}

func assertNotCalled(t *testing.T, calls []string, method string) {
	t.Helper()
	for _, c := range calls {
		if c == method {
			t.Errorf("expected %q NOT to be called; got %v", method, calls)
			return
		}
	}
}

func indexOf(calls []string, method string) int {
	for i, c := range calls {
		if c == method {
			return i
		}
	}
	return -1
}

// Tests

func TestRun_noArgs(t *testing.T) {
	app := newTestApp(newMockScale())
	if err := app.Run(context.Background(), nil); err == nil {
		t.Error("expected error when no args provided")
	}
}

func TestRun_unknownCommand(t *testing.T) {
	app := newTestApp(newMockScale())
	if err := app.Run(context.Background(), []string{"bogus"}); err == nil {
		t.Error("expected error for unknown command")
	}
}

func TestTare(t *testing.T) {
	mock := newMockScale()
	sendMeasurement(mock)

	if err := newTestApp(mock).Run(context.Background(), []string{"tare"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertCalled(t, mock.called(), "Tare")
}

func TestStart(t *testing.T) {
	mock := newMockScale()
	sendMeasurement(mock)

	if err := newTestApp(mock).Run(context.Background(), []string{"start"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertCalled(t, mock.called(), "StartTimer")
}

func TestStop(t *testing.T) {
	mock := newMockScale()
	sendMeasurement(mock)

	if err := newTestApp(mock).Run(context.Background(), []string{"stop"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertCalled(t, mock.called(), "StopTimer")
}

func TestReset_stopsBeforeResetting(t *testing.T) {
	mock := newMockScale()
	sendMeasurement(mock)

	if err := newTestApp(mock).Run(context.Background(), []string{"reset"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := mock.called()
	assertCalled(t, calls, "StopTimer")
	assertCalled(t, calls, "ResetTimer")

	if stopIdx, resetIdx := indexOf(calls, "StopTimer"), indexOf(calls, "ResetTimer"); stopIdx > resetIdx {
		t.Errorf("StopTimer must come before ResetTimer; got calls: %v", calls)
	}
}

func TestShot_callsTareAndStart(t *testing.T) {
	mock := newMockScale()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so stream exits without blocking

	newTestApp(mock).Run(ctx, []string{"shot"})

	assertCalled(t, mock.called(), "TareAndStart")
}

func TestShot_stopsTimerOnCancel(t *testing.T) {
	mock := newMockScale()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	newTestApp(mock).Run(ctx, []string{"shot"})

	assertCalled(t, mock.called(), "StopTimer")
}

func TestShot_doesNotStopTimerOnScaleDisconnect(t *testing.T) {
	mock := newMockScale()
	mock.disconnect() // simulate the scale powering off

	newTestApp(mock).Run(context.Background(), []string{"shot"})

	// When the scale disconnects we don't try to stop the timer.
	assertCalled(t, mock.called(), "TareAndStart")
	assertNotCalled(t, mock.called(), "StopTimer")
}

func TestMonitor_exitsOnScaleDisconnect(t *testing.T) {
	mock := newMockScale()
	mock.disconnect()

	if err := newTestApp(mock).Run(context.Background(), []string{"monitor"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCommand_propagatesError(t *testing.T) {
	mock := newMockScale()
	mock.errs["Tare"] = errors.New("scale busy")

	err := newTestApp(mock).Run(context.Background(), []string{"tare"})
	if err == nil {
		t.Error("expected error when Tare fails")
	}
}

func TestReset_propagatesStopError(t *testing.T) {
	mock := newMockScale()
	mock.errs["StopTimer"] = errors.New("comms error")

	err := newTestApp(mock).Run(context.Background(), []string{"reset"})
	if err == nil {
		t.Error("expected error when StopTimer fails")
	}
	// ResetTimer should not be called if StopTimer failed.
	assertNotCalled(t, mock.called(), "ResetTimer")
}

func TestClose_alwaysCalled(t *testing.T) {
	for _, cmd := range []string{"tare", "start", "stop"} {
		t.Run(cmd, func(t *testing.T) {
			mock := newMockScale()
			sendMeasurement(mock)
			newTestApp(mock).Run(context.Background(), []string{cmd})
			assertCalled(t, mock.called(), "Close")
		})
	}
}
