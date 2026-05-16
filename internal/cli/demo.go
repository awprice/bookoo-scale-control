package cli

import (
	"math"
	"sync"
	"time"

	"github.com/awprice/bookoo-scale-control/pkg/bookoo"
)

// demoScale implements Scale using pre-generated fake shot data, streaming
// measurements at 10 Hz to simulate a real espresso shot without Bluetooth.
type demoScale struct {
	measurements chan bookoo.Measurement
	stop         chan struct{}
	once         sync.Once
}

func newDemoScale() *demoScale {
	return &demoScale{
		measurements: make(chan bookoo.Measurement, 16),
		stop:         make(chan struct{}),
	}
}

func (d *demoScale) TareAndStart() error {
	go func() {
		defer close(d.measurements)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for _, m := range fakeShotMeasurements() {
			select {
			case <-d.stop:
				return
			case <-ticker.C:
				d.measurements <- m
			}
		}
	}()
	return nil
}

func (d *demoScale) Measurements() <-chan bookoo.Measurement { return d.measurements }

func (d *demoScale) Close() error {
	d.once.Do(func() { close(d.stop) })
	return nil
}

func (d *demoScale) Tare() error                                    { return nil }
func (d *demoScale) StartTimer() error                              { return nil }
func (d *demoScale) StopTimer() error                               { return nil }
func (d *demoScale) ResetTimer() error                              { return nil }
func (d *demoScale) Calibrate() error                               { return nil }
func (d *demoScale) SetBeepLevel(int) error                         { return nil }
func (d *demoScale) SetAutoOff(int) error                           { return nil }
func (d *demoScale) SetFlowSmoothing(bool) error                    { return nil }
func (d *demoScale) SetStopCondition(bookoo.StopCondition) error    { return nil }

// fakeShotMeasurements generates a deterministic 30-second espresso shot profile
// with ~40g yield, including a 5-second pre-infusion phase where no liquid
// reaches the cup and realistic flow rate variation during extraction.
func fakeShotMeasurements() []bookoo.Measurement {
	const dt = 0.1 // 10 Hz
	out := make([]bookoo.Measurement, 0, 301)
	weight := 0.0

	for i := 0; i <= 300; i++ {
		t := float64(i) * dt

		var flow float64
		switch {
		case t < 5:
			// Pre-infusion: pump soaks the puck; no liquid reaches the cup yet.
			flow = 0
		case t < 9:
			// First drops, ramping up to full extraction speed.
			flow = 0.475 * (t - 5) // 0 → 1.9 g/s over 4 s
		case t < 21:
			// Main extraction with a natural flow arc and minor channelling wobble.
			flow = 1.95 + 0.15*math.Sin(math.Pi*(t-9)/12) + 0.07*math.Sin(5.1*(t-9))
		default:
			// Gradual taper as the puck dries out.
			flow = 1.85 - 0.135*(t-21) + 0.04*math.Sin(3.3*(t-21))
			if flow < 0.1 {
				flow = 0.1
			}
		}

		weight += flow * dt
		out = append(out, bookoo.Measurement{
			Weight:    math.Round(weight*100) / 100,
			FlowRate:  math.Round(flow*100) / 100,
			Timestamp: time.Duration(i) * 100 * time.Millisecond,
			Battery:   85,
		})
	}
	return out
}
