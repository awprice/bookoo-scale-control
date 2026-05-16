package bookoo

import (
	"testing"
	"time"
)

// buildPacket constructs a minimal valid 20-byte weight notification packet.
func buildPacket(tsMs uint32, unit WeightUnit, wSign byte, weightRaw uint32, fSign byte, flowRaw uint16, battery uint8) []byte {
	pkt := make([]byte, 20)
	pkt[0] = 0x03
	pkt[1] = 0x0B
	pkt[2] = byte(tsMs >> 16)
	pkt[3] = byte(tsMs >> 8)
	pkt[4] = byte(tsMs)
	pkt[5] = byte(unit)
	pkt[6] = wSign
	pkt[7] = byte(weightRaw >> 16)
	pkt[8] = byte(weightRaw >> 8)
	pkt[9] = byte(weightRaw)
	pkt[10] = fSign
	pkt[11] = byte(flowRaw >> 8)
	pkt[12] = byte(flowRaw)
	pkt[13] = battery
	return pkt
}

func TestParsePacket_positiveWeight(t *testing.T) {
	// 250.00 g, 2.50 g/s, 72% battery, 30s elapsed
	pkt := buildPacket(30_000, WeightUnitGram, 0x00, 25_000, 0x00, 250, 72)
	m, ok := parsePacket(pkt)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if m.Weight != 250.0 {
		t.Errorf("Weight: got %v, want 250.0", m.Weight)
	}
	if m.FlowRate != 2.5 {
		t.Errorf("FlowRate: got %v, want 2.5", m.FlowRate)
	}
	if m.Battery != 72 {
		t.Errorf("Battery: got %v, want 72", m.Battery)
	}
	if m.Timestamp != 30*time.Second {
		t.Errorf("Timestamp: got %v, want 30s", m.Timestamp)
	}
	if m.Unit != WeightUnitGram {
		t.Errorf("Unit: got %v, want WeightUnitGram", m.Unit)
	}
}

func TestParsePacket_negativeWeight(t *testing.T) {
	// -12.50 g (below tare)
	pkt := buildPacket(5_000, WeightUnitGram, 0x2D, 1_250, 0x00, 0, 85)
	m, ok := parsePacket(pkt)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if m.Weight != -12.5 {
		t.Errorf("Weight: got %v, want -12.5", m.Weight)
	}
}

func TestParsePacket_negativeFlow(t *testing.T) {
	// Flow rate negative (weight decreasing)
	pkt := buildPacket(1_000, WeightUnitGram, 0x00, 500, 0x2D, 75, 90)
	m, ok := parsePacket(pkt)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if m.FlowRate != -0.75 {
		t.Errorf("FlowRate: got %v, want -0.75", m.FlowRate)
	}
}

func TestParsePacket_unitOunce(t *testing.T) {
	pkt := buildPacket(0, WeightUnitOunce, 0x00, 0, 0x00, 0, 100)
	m, ok := parsePacket(pkt)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if m.Unit != WeightUnitOunce {
		t.Errorf("Unit: got %v, want WeightUnitOunce", m.Unit)
	}
}

func TestParsePacket_zeroWeight(t *testing.T) {
	pkt := buildPacket(0, WeightUnitGram, 0x00, 0, 0x00, 0, 50)
	m, ok := parsePacket(pkt)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if m.Weight != 0 {
		t.Errorf("Weight: got %v, want 0", m.Weight)
	}
}

func TestParsePacket_tooShort(t *testing.T) {
	_, ok := parsePacket([]byte{0x03, 0x0B, 0x00})
	if ok {
		t.Error("expected ok=false for short packet")
	}
}

func TestParsePacket_wrongHeader(t *testing.T) {
	pkt := make([]byte, 20)
	pkt[0] = 0xFF // wrong
	pkt[1] = 0x0B
	_, ok := parsePacket(pkt)
	if ok {
		t.Error("expected ok=false for wrong header")
	}
}

func TestParsePacket_wrongType(t *testing.T) {
	pkt := make([]byte, 20)
	pkt[0] = 0x03
	pkt[1] = 0xFF // wrong
	_, ok := parsePacket(pkt)
	if ok {
		t.Error("expected ok=false for wrong type byte")
	}
}

func TestBuildCmd_checksumIsXOROfAllBytes(t *testing.T) {
	cases := []struct{ d1, d2, d3 byte }{
		{0x02, 0x00, 0x00}, // beep silent
		{0x02, 0x00, 0x05}, // beep loudest
		{0x03, 0x00, 0x05}, // auto-off 5 min
		{0x03, 0x00, 0x1E}, // auto-off 30 min
		{0x08, 0x01, 0x00}, // flow smoothing on
		{0x08, 0x00, 0x00}, // flow smoothing off
		{0x09, 0x00, 0x00}, // calibrate
		{0x0B, 0x00, 0x00}, // stop condition: flow stops
		{0x0B, 0x01, 0x00}, // stop condition: container removed
	}
	for _, tc := range cases {
		cmd := buildCmd(tc.d1, tc.d2, tc.d3)
		if len(cmd) != 6 {
			t.Fatalf("buildCmd returned %d bytes, want 6", len(cmd))
		}
		want := cmd[0] ^ cmd[1] ^ cmd[2] ^ cmd[3] ^ cmd[4]
		if cmd[5] != want {
			t.Errorf("buildCmd(0x%02X,0x%02X,0x%02X): checksum 0x%02X, want 0x%02X",
				tc.d1, tc.d2, tc.d3, cmd[5], want)
		}
	}
}

func TestBuildCmd_calibrateChecksumMatchesDoc(t *testing.T) {
	// The Ultra protocol documentation explicitly states the calibrate checksum is 0x00,
	// which we can independently verify with the XOR formula.
	cmd := buildCmd(0x09, 0x00, 0x00)
	if cmd[5] != 0x00 {
		t.Errorf("calibrate checksum: got 0x%02X, want 0x00", cmd[5])
	}
}

func TestCommandBytes(t *testing.T) {
	cases := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"tare", cmdTare, []byte{0x03, 0x0A, 0x01, 0x00, 0x00, 0x08}},
		{"timer start", cmdTimerStart, []byte{0x03, 0x0A, 0x04, 0x00, 0x00, 0x0A}},
		{"timer stop", cmdTimerStop, []byte{0x03, 0x0A, 0x05, 0x00, 0x00, 0x0D}},
		{"timer reset", cmdTimerReset, []byte{0x03, 0x0A, 0x06, 0x00, 0x00, 0x0C}},
		{"tare and start", cmdTareAndStart, []byte{0x03, 0x0A, 0x07, 0x00, 0x00, 0x00}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.got) != len(tc.want) {
				t.Fatalf("length mismatch: got %d, want %d", len(tc.got), len(tc.want))
			}
			for i := range tc.want {
				if tc.got[i] != tc.want[i] {
					t.Errorf("byte[%d]: got 0x%02X, want 0x%02X", i, tc.got[i], tc.want[i])
				}
			}
		})
	}
}
