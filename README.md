# bookoo-scale-control

A Go library and CLI for interacting with [Bookoo](https://bookoocoffee.com) espresso scales over Bluetooth Low Energy.

## Supported Scales

| Scale | Status |
|---|---|
| Bookoo Themis Ultra | Tested |
| Bookoo Mini | Supported (untested) |

## CLI

### Install

```sh
go install github.com/awprice/bookoo-scale-control/cmd/bookoo@latest
```

### Usage

```
bookoo <command> [flags]

Commands:
  monitor    Stream live weight measurements until Ctrl+C
  tare       Tare the scale (zero the weight)
  shot       Tare and start the timer (espresso shot mode)
  start      Start the built-in timer
  stop       Stop the built-in timer
  reset      Reset the built-in timer to zero

Flags:
  -timeout duration   How long to scan before giving up (default 30s)
```

### Examples

```sh
# Watch live measurements while pulling a shot
bookoo monitor

# At the start of a shot: tare and start the timer in one step
bookoo shot

# Stop and reset the timer when the shot is done
bookoo stop
bookoo reset
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
| `Close()` | Disconnect and release resources |

Each `Measurement` contains:

```go
type Measurement struct {
    Weight    float64       // grams (negative if below tare)
    FlowRate  float64       // grams/second
    Battery   int           // 0–100
    Timestamp time.Duration // scale's internal elapsed time
    Unit      WeightUnit    // gram or ounce
}
```

## Requirements

- macOS with Bluetooth LE
- Go 1.21+
- Xcode Command Line Tools (CGo required by the BLE library)

## Protocol Sources

- [Bookoo Ultra Scale Protocol](https://github.com/BooKooCode/OpenSource/blob/main/bookoo_ultra_scale/protocols.md) — official BLE protocol specification
- [Bookoo Mini Scale Protocol](https://github.com/BooKooCode/OpenSource/blob/main/bookoo_mini_scale/protocols.md) — official BLE protocol specification
- [AcaiaArduinoBLE](https://github.com/tatemazer/AcaiaArduinoBLE) — reference implementation for Bookoo generic scale commands

## License

MIT
