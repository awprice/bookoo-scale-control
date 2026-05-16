package bookoo

import (
	"time"

	"tinygo.org/x/bluetooth"
)

var (
	serviceUUID    = bluetooth.New16BitUUID(0x0FFE)
	writeCharUUID  = bluetooth.New16BitUUID(0xFF12)
	notifyCharUUID = bluetooth.New16BitUUID(0xFF11)
)

// Commands are 6 bytes: [product, type, data1, data2, data3, checksum].
// Fixed commands use byte values taken directly from the official Bookoo protocol
// documentation. Parameterised commands are built by buildCmd.
var (
	cmdTare         = []byte{0x03, 0x0A, 0x01, 0x00, 0x00, 0x08}
	cmdTimerStart   = []byte{0x03, 0x0A, 0x04, 0x00, 0x00, 0x0A}
	cmdTimerStop    = []byte{0x03, 0x0A, 0x05, 0x00, 0x00, 0x0D}
	cmdTimerReset   = []byte{0x03, 0x0A, 0x06, 0x00, 0x00, 0x0C}
	cmdTareAndStart = []byte{0x03, 0x0A, 0x07, 0x00, 0x00, 0x00}
)

// buildCmd constructs a 6-byte command packet for parameterised commands.
// The checksum is computed as the XOR of all preceding bytes per the protocol spec.
func buildCmd(data1, data2, data3 byte) []byte {
	checksum := byte(0x03) ^ byte(0x0A) ^ data1 ^ data2 ^ data3
	return []byte{0x03, 0x0A, data1, data2, data3, checksum}
}

// StopCondition controls when the scale stops automatically in auto mode.
type StopCondition uint8

const (
	// StopConditionFlowStops halts the timer when the flow rate reaches zero.
	StopConditionFlowStops StopCondition = 0x00
	// StopConditionContainerRemoved halts the timer when the container is lifted off.
	StopConditionContainerRemoved StopCondition = 0x01
)

// WeightUnit is the unit of measurement reported by the scale.
type WeightUnit uint8

const (
	WeightUnitOunce WeightUnit = 1
	WeightUnitGram  WeightUnit = 2
)

// Measurement is a single reading received from the scale via BLE notification.
type Measurement struct {
	// Weight in grams. Negative values indicate a reading below the tare point.
	Weight float64
	// FlowRate in grams per second. Negative indicates decreasing weight.
	FlowRate float64
	// Battery charge level, 0–100.
	Battery int
	// Timestamp is the scale's internal elapsed time for the current session.
	Timestamp time.Duration
	// Unit is the weight unit currently configured on the scale.
	Unit WeightUnit
}

// parsePacket decodes a 20-byte BLE notification into a Measurement.
// Returns false if the packet is malformed or not a weight notification.
func parsePacket(pkt []byte) (Measurement, bool) {
	if len(pkt) < 20 || pkt[0] != 0x03 || pkt[1] != 0x0B {
		return Measurement{}, false
	}

	tsMs := uint32(pkt[2])<<16 | uint32(pkt[3])<<8 | uint32(pkt[4])

	weightRaw := int32(pkt[7])<<16 | int32(pkt[8])<<8 | int32(pkt[9])
	weight := float64(weightRaw) / 100.0
	if pkt[6] == 0x2D {
		weight = -weight
	}

	flowRaw := int32(pkt[11])<<8 | int32(pkt[12])
	flow := float64(flowRaw) / 100.0
	if pkt[10] == 0x2D {
		flow = -flow
	}

	return Measurement{
		Weight:    weight,
		FlowRate:  flow,
		Battery:   int(pkt[13]),
		Timestamp: time.Duration(tsMs) * time.Millisecond,
		Unit:      WeightUnit(pkt[5]),
	}, true
}
