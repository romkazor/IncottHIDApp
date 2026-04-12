package main

import "testing"

func TestLodLabel(t *testing.T) {
	tests := []struct {
		lod  int
		want string
	}{
		{7, "0.7mm"},
		{10, "1mm"},
		{20, "2mm"},
		{30, "3mm"}, // fallback formatter
	}
	for _, tt := range tests {
		got := lodLabel(tt.lod)
		if got != tt.want {
			t.Errorf("lodLabel(%d) = %q, want %q", tt.lod, got, tt.want)
		}
	}
}

func TestSleepLabel(t *testing.T) {
	tests := []struct {
		sec  int
		want string
	}{
		{10, "10s"},
		{30, "30s"},
		{59, "59s"},
		{60, "1m"},
		{120, "2m"},
		{900, "15m"},
	}
	for _, tt := range tests {
		got := sleepLabel(tt.sec)
		if got != tt.want {
			t.Errorf("sleepLabel(%d) = %q, want %q", tt.sec, got, tt.want)
		}
	}
}
