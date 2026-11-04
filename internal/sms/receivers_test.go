package sms

import (
	"reflect"
	"testing"
)

func TestNormalizeNumber(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"06 12 34 56 78", "+33612345678", true},
		{"0612345678", "+33612345678", true},
		{"+33 6 12 34 56 78", "+33612345678", true},
		{"0033612345678", "+33612345678", true},
		{"+33 (0)6 12 34 56 78", "+33612345678", true},
		{"06.12.34.56.78", "+33612345678", true},
		{"+14155552671", "+14155552671", true},
		{"12345", "12345", false},
		{"abc", "abc", false},
		{"+33", "+33", false},
		{"06123", "+336123", false},
	}
	for _, tc := range tests {
		got, ok := NormalizeNumber(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("NormalizeNumber(%q) = %q,%v ; want %q,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestParseReceivers(t *testing.T) {
	r := ParseReceivers("06 12 34 56 78, +33612345678;\n0698765432\n\nfoo\n")
	want := Receivers{
		Valid:   []string{"+33612345678", "+33698765432"},
		Invalid: []string{"foo"},
		Dupes:   1,
	}
	if !reflect.DeepEqual(r, want) {
		t.Fatalf("got %+v, want %+v", r, want)
	}
	if e := ParseReceivers(""); len(e.Valid) != 0 || e.Valid == nil {
		t.Fatalf("empty = %+v", e)
	}
}
