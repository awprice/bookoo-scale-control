// Package bookoo provides a client for Bookoo espresso scales over Bluetooth Low Energy.
//
// Supported scales: Bookoo Themis Ultra, Bookoo Mini.
//
// Basic usage:
//
//	scale, err := bookoo.Connect(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer scale.Close()
//
//	for m := range scale.Measurements() {
//	    fmt.Printf("%.2f g\n", m.Weight)
//	}
package bookoo

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"tinygo.org/x/bluetooth"
)

// Scale represents a connected Bookoo scale.
// Call Close when done to disconnect and release resources.
type Scale struct {
	device       bluetooth.Device
	writeChar    bluetooth.DeviceCharacteristic
	measurements chan Measurement
	mu           sync.RWMutex
	isClosed     bool
	closeOnce    sync.Once
}

// Connect scans for the nearest Bookoo scale and connects to it.
// Cancel ctx to abort the scan; a deadline on ctx controls the scan timeout.
// Both the Bookoo Themis Ultra and Bookoo Mini are supported.
func Connect(ctx context.Context) (*Scale, error) {
	adapter := bluetooth.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		return nil, fmt.Errorf("enable BLE adapter: %w", err)
	}

	found := make(chan bluetooth.ScanResult, 1)
	scanDone := make(chan error, 1)

	go func() {
		err := adapter.Scan(func(a *bluetooth.Adapter, result bluetooth.ScanResult) {
			if strings.HasPrefix(strings.ToLower(result.LocalName()), "bookoo") {
				select {
				case found <- result:
					a.StopScan()
				default:
				}
			}
		})
		scanDone <- err
	}()

	var result bluetooth.ScanResult
	select {
	case result = <-found:
		<-scanDone
	case err := <-scanDone:
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		return nil, fmt.Errorf("scan ended without finding a Bookoo scale")
	case <-ctx.Done():
		adapter.StopScan()
		<-scanDone
		return nil, ctx.Err()
	}

	device, err := adapter.Connect(result.Address, bluetooth.ConnectionParams{})
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", result.LocalName(), err)
	}

	services, err := device.DiscoverServices([]bluetooth.UUID{serviceUUID})
	if err != nil {
		device.Disconnect()
		return nil, fmt.Errorf("discover services: %w", err)
	}
	if len(services) == 0 {
		device.Disconnect()
		return nil, fmt.Errorf("Bookoo BLE service (0x0FFE) not found on device")
	}

	chars, err := services[0].DiscoverCharacteristics(nil)
	if err != nil {
		device.Disconnect()
		return nil, fmt.Errorf("discover characteristics: %w", err)
	}

	s := &Scale{
		device:       device,
		measurements: make(chan Measurement, 16),
	}

	var writeFound, notifyFound bool
	for _, c := range chars {
		switch c.UUID() {
		case writeCharUUID:
			s.writeChar = c
			writeFound = true
		case notifyCharUUID:
			if err := c.EnableNotifications(s.handleNotification); err != nil {
				device.Disconnect()
				return nil, fmt.Errorf("enable notifications: %w", err)
			}
			notifyFound = true
		}
	}

	if !writeFound || !notifyFound {
		device.Disconnect()
		return nil, fmt.Errorf("required BLE characteristics not found (write=%v notify=%v)", writeFound, notifyFound)
	}

	return s, nil
}

func (s *Scale) handleNotification(buf []byte) {
	m, ok := parsePacket(buf)
	if !ok {
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.isClosed {
		return
	}
	select {
	case s.measurements <- m:
	default:
	}
}

func (s *Scale) writeCmd(cmd []byte) error {
	_, err := s.writeChar.Write(cmd)
	return err
}

// Tare zeros the current weight reading.
func (s *Scale) Tare() error { return s.writeCmd(cmdTare) }

// StartTimer starts the built-in timer.
func (s *Scale) StartTimer() error { return s.writeCmd(cmdTimerStart) }

// StopTimer stops the built-in timer.
func (s *Scale) StopTimer() error { return s.writeCmd(cmdTimerStop) }

// ResetTimer resets the built-in timer to zero.
func (s *Scale) ResetTimer() error { return s.writeCmd(cmdTimerReset) }

// TareAndStart tares the scale and starts the timer in a single operation.
// This is the recommended command to use at the start of an espresso shot.
func (s *Scale) TareAndStart() error { return s.writeCmd(cmdTareAndStart) }

// Measurements returns a channel of live weight readings from the scale.
// The channel is closed when Close is called.
func (s *Scale) Measurements() <-chan Measurement {
	return s.measurements
}

// Close disconnects from the scale and closes the Measurements channel.
// It is safe to call Close multiple times.
func (s *Scale) Close() error {
	var disconnectErr error
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.isClosed = true
		close(s.measurements)
		s.mu.Unlock()
		disconnectErr = s.device.Disconnect()
	})
	return disconnectErr
}
