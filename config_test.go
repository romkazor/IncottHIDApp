package main

import (
	"reflect"
	"testing"
)

func TestEscapePowerShellSingleQuoted(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"cs2.exe", "cs2.exe"},
		{"", ""},
		{"it's", "it''s"},
		{"'; calc; '", "''; calc; ''"},
		{"'''", "''''''"},
	}
	for _, tt := range tests {
		got := escapePowerShellSingleQuoted(tt.in)
		if got != tt.want {
			t.Errorf("escapePowerShellSingleQuoted(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseTargetApps(t *testing.T) {
	tests := []struct {
		raw  string
		want []string
	}{
		{"", []string{}},
		{"cs2.exe", []string{"cs2.exe"}},
		{"cs2.exe,dota2.exe", []string{"cs2.exe", "dota2.exe"}},
		{"cs2.exe, dota2.exe, valorant.exe", []string{"cs2.exe", "dota2.exe", "valorant.exe"}},
		{"CS2.EXE", []string{"cs2.exe"}}, // lowercased
		{"cs2", []string{"cs2.exe"}},     // .exe appended
		{"  cs2  , dota2  ", []string{"cs2.exe", "dota2.exe"}}, // trimmed + .exe
		{",,,", []string{}}, // empty parts skipped
		{"valid.exe,,empty", []string{"valid.exe", "empty.exe"}},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got := parseTargetApps(tt.raw)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseTargetApps(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}
