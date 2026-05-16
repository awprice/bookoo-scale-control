# bookoo-scale-control

A Go library and CLI for interacting with [Bookoo](https://bookoocoffee.com) espresso scales over Bluetooth Low Energy.

![Demo](docs/demo.gif)

## Supported Scales

| Scale | Status |
|---|---|
| Bookoo Themis Ultra | Tested |
| Bookoo Themis Mini | Supported (untested) |

## CLI

### Install

```sh
go install github.com/awprice/bookoo-scale-control/cmd/bookoo@latest
```

### Usage

```
bookoo <command> [flags]

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
```

### Examples

```sh
# Pull a shot: tares, starts the timer, and shows real-time weight and flow rate charts.
# Ctrl+C stops the timer and leaves the final chart on screen as a shot summary.
bookoo shot

# Watch live measurements without touching the timer
bookoo monitor

# Reset the timer between shots (stops it first if still running)
bookoo reset

# Read current settings
bookoo settings

# Adjust settings
bookoo beep 3               # set volume to mid-level
bookoo auto-off 10          # power off after 10 minutes idle
bookoo smoothing on         # enable flow rate smoothing
bookoo stop-condition flow  # stop timer when flow drops to zero (Ultra only)

# Try the CLI without a scale
bookoo demo
```

## Library

```sh
go get github.com/awprice/bookoo-scale-control/pkg/bookoo
```

```go
import "github.com/awprice/bookoo-scale-control/pkg/bookoo"

scale, err := bookoo.Connect(ctx)
if err != nil {
    log.Fatal(err)
}
defer scale.Close()

// Stream measurements
for m := range scale.Measurements() {
    fmt.Printf("%.2f g  (%.2f g/s)\n", m.Weight, m.FlowRate)
}
```

### API

| Method | Description |
|---|---|
| `Connect(ctx)` | Scan for and connect to the nearest Bookoo scale |
| `Measurements()` | Channel of live `Measurement` readings |
| `Tare()` | Zero the weight |
| `TareAndStart()` | Tare + start timer (espresso shot mode) |
| `StartTimer()` | Start the built-in timer |
| `StopTimer()` | Stop the built-in timer |
| `ResetTimer()` | Reset the timer to zero |
| `SetBeepLevel(level int)` | Set speaker volume (0=silent, 5=loudest) |
| `SetAutoOff(minutes int)` | Set inactivity auto-off timeout (5–30 minutes) |
| `SetFlowSmoothing(enabled bool)` | Enable or disable flow rate smoothing |
| `Calibrate()` | Run the built-in calibration routine (scale must be empty) |
| `SetStopCondition(cond)` | Set auto-stop trigger (`StopConditionFlowStops` or `StopConditionContainerRemoved`) |
| `Close()` | Disconnect and release resources |

Each `Measurement` contains:

```go
type Measurement struct {
    // Live readings
    Weight    float64       // grams (negative if below tare)
    FlowRate  float64       // grams/second
    Battery   int           // 0–100
    Timestamp time.Duration // scale's internal elapsed time
    Unit      WeightUnit    // gram or ounce

    // Current settings (reported by the scale in every packet)
    AutoOff       int           // inactivity auto-off timeout in minutes
    BeepLevel     int           // speaker volume, 0 (silent) to 5 (loudest)
    FlowSmoothing bool          // whether flow rate smoothing is enabled
    StopCondition StopCondition // auto-stop trigger mode (Themis Ultra)
}
```

## Requirements

- macOS with Bluetooth LE
- Go 1.21+
- Xcode Command Line Tools (CGo required by the BLE library)
- A 256-colour terminal for the shot graph (most modern terminals qualify)

## Protocol Sources

- [Bookoo Ultra Scale Protocol](https://github.com/BooKooCode/OpenSource/blob/main/bookoo_ultra_scale/protocols.md) — official BLE protocol specification
- [Bookoo Themis Mini Scale Protocol](https://github.com/BooKooCode/OpenSource/blob/main/bookoo_mini_scale/protocols.md) — official BLE protocol specification
- [AcaiaArduinoBLE](https://github.com/tatemazer/AcaiaArduinoBLE) — reference implementation for Bookoo generic scale commands

## License

MIT
