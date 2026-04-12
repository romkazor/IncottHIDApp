package main

import "testing"

func TestIsMouseDevice(t *testing.T) {
	tests := []struct {
		product string
		want    bool
	}{
		{"", true}, // empty = assume mouse
		{"incott 8K wireless mouse", true},
		{"Incott G23 Wireless", true},
		{"G24 Pro", true},
		{"Ghero Series Mouse", true},
		{"Zero 29 Wireless", true},
		{"Zero 39 Pro", true},
		{"Incott Keyboard", false},
		{"Some Other Device", false},
		{"MOUSE", true}, // case insensitive
		{"g23v2", true},
	}
	for _, tt := range tests {
		t.Run(tt.product, func(t *testing.T) {
			got := isMouseDevice(tt.product)
			if got != tt.want {
				t.Errorf("isMouseDevice(%q) = %v, want %v", tt.product, got, tt.want)
			}
		})
	}
}

func TestParseLODByte(t *testing.T) {
	tests := []struct {
		b    byte
		want int
	}{
		{0x01, 10}, // 1mm + motion sync on
		{0x00, 10}, // 1mm + motion sync off
		{0x11, 20}, // 2mm + motion sync on
		{0x10, 20}, // 2mm + motion sync off
		{0x21, 7},  // 0.7mm + motion sync on
		{0x20, 7},  // 0.7mm + motion sync off
		{0xF0, 0},  // unknown upper nibble
	}
	for _, tt := range tests {
		got := parseLODByte(tt.b)
		if got != tt.want {
			t.Errorf("parseLODByte(0x%02X) = %d, want %d", tt.b, got, tt.want)
		}
	}
}

func TestParseMotionSyncByte(t *testing.T) {
	tests := []struct {
		b    byte
		want int
	}{
		{0x00, 0},
		{0x01, 1},
		{0x10, 0},
		{0x11, 1},
		{0x21, 1},
		{0x22, 0},
	}
	for _, tt := range tests {
		got := parseMotionSyncByte(tt.b)
		if got != tt.want {
			t.Errorf("parseMotionSyncByte(0x%02X) = %d, want %d", tt.b, got, tt.want)
		}
	}
}

func TestParseSleepBytes(t *testing.T) {
	tests := []struct {
		lo, hi byte
		want   int
	}{
		{0x0A, 0x00, 10},  // 10s
		{0x3C, 0x00, 60},  // 60s
		{0x78, 0x00, 120}, // 120s
		{0x2C, 0x01, 300}, // 300s
		{0x84, 0x03, 900}, // 900s (max)
		{0x00, 0x00, 0},   // invalid
		{0xFF, 0xFF, 0},   // out of range
	}
	for _, tt := range tests {
		got := parseSleepBytes(tt.lo, tt.hi)
		if got != tt.want {
			t.Errorf("parseSleepBytes(0x%02X, 0x%02X) = %d, want %d", tt.lo, tt.hi, got, tt.want)
		}
	}
}

func TestParseStatus(t *testing.T) {
	tests := []struct {
		name        string
		buf         []byte
		wantBat     int16
		wantDPI     int
		wantHz      int
	}{
		{
			name:    "normal: 80% bat, 800 DPI, 1000 Hz",
			buf:     []byte{0x09, 80, 0x10}, // dpi preset 1, hz preset 0
			wantBat: 80, wantDPI: 800, wantHz: 1000,
		},
		{
			name:    "charging bit: 137 -> 9%",
			buf:     []byte{0x09, 137, 0x10},
			wantBat: 9, wantDPI: 800, wantHz: 1000,
		},
		{
			name:    "8000 Hz (preset 4) + 6400 DPI (preset 5)",
			buf:     []byte{0x09, 50, 0x54},
			wantBat: 50, wantDPI: 6400, wantHz: 8000,
		},
		{
			name:    "buf too short",
			buf:     []byte{0x09},
			wantBat: 0, wantDPI: 0, wantHz: 0,
		},
		{
			name:    "out of range presets fall back to defaults",
			buf:     []byte{0x09, 60, 0xFF},
			wantBat: 60, wantDPI: 800, wantHz: 1000,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bat, dpi, hz := parseStatus(tt.buf)
			if bat != tt.wantBat || dpi != tt.wantDPI || hz != tt.wantHz {
				t.Errorf("parseStatus(%v) = (%d, %d, %d), want (%d, %d, %d)",
					tt.buf, bat, dpi, hz, tt.wantBat, tt.wantDPI, tt.wantHz)
			}
		})
	}
}

func TestParseDebounceByte(t *testing.T) {
	tests := []struct {
		b    byte
		want int
	}{
		{0, 0},
		{1, 1},
		{8, 8},
		{30, 30},
		{31, -1}, // out of range
		{0xFF, -1},
	}
	for _, tt := range tests {
		got := parseDebounceByte(tt.b)
		if got != tt.want {
			t.Errorf("parseDebounceByte(0x%02X) = %d, want %d", tt.b, got, tt.want)
		}
	}
}

func TestParseToggleByte(t *testing.T) {
	tests := []struct {
		b    byte
		want int
	}{
		{0x00, 0},
		{0x01, 1},
		{0x02, 0}, // anything other than 0x01 = off
		{0xFF, 0},
	}
	for _, tt := range tests {
		got := parseToggleByte(tt.b)
		if got != tt.want {
			t.Errorf("parseToggleByte(0x%02X) = %d, want %d", tt.b, got, tt.want)
		}
	}
}

func TestParseReceiverLEDByte(t *testing.T) {
	tests := []struct {
		b    byte
		want int
	}{
		{0, 0},
		{1, 1},
		{2, 2},
		{3, -1}, // out of range
		{0xFF, -1},
	}
	for _, tt := range tests {
		got := parseReceiverLEDByte(tt.b)
		if got != tt.want {
			t.Errorf("parseReceiverLEDByte(0x%02X) = %d, want %d", tt.b, got, tt.want)
		}
	}
}

func TestPresets(t *testing.T) {
	tests := []struct {
		name        string
		dpiIdx      int
		hzIdx       int
		wantDPI     int
		wantHz      int
	}{
		{"defaults", -1, -1, 800, 1000},
		{"first", 0, 0, 400, 1000},
		{"800/500", 1, 1, 800, 500},
		{"6400/2000", 5, 6, 6400, 2000},
		{"out of range", 99, 99, 800, 1000},
		{"8000Hz", 0, 4, 400, 8000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dpi, hz := presets(tt.dpiIdx, tt.hzIdx)
			if dpi != tt.wantDPI || hz != tt.wantHz {
				t.Errorf("presets(%d, %d) = (%d, %d), want (%d, %d)",
					tt.dpiIdx, tt.hzIdx, dpi, hz, tt.wantDPI, tt.wantHz)
			}
		})
	}
}
