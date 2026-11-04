package sms

import (
	"strings"
	"testing"
)

func TestAnalyze(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		enc      Encoding
		segments int
		units    int
	}{
		{"vide", "", GSM7, 0, 0},
		{"court", "Bonjour", GSM7, 1, 7},
		{"160 gsm", strings.Repeat("a", 160), GSM7, 1, 160},
		{"161 gsm", strings.Repeat("a", 161), GSM7, 2, 161},
		{"306 gsm", strings.Repeat("a", 306), GSM7, 2, 306},
		{"307 gsm", strings.Repeat("a", 307), GSM7, 3, 307},
		{"extension compte double", "€", GSM7, 1, 2},
		{"accents gsm", "éèà", GSM7, 1, 3},
		{"ucs2 emoji", "Salut 😀", UCS2, 1, 9},
		{"70 ucs2", strings.Repeat("→", 70), UCS2, 1, 70},
		{"ucs2 multi", strings.Repeat("→", 71), UCS2, 2, 71},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := Analyze(tc.msg)
			if a.Encoding != tc.enc || a.Segments != tc.segments {
				t.Fatalf("got %+v, want enc=%s segments=%d", a, tc.enc, tc.segments)
			}
		})
	}
}
