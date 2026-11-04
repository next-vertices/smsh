// Package sms contient les règles pures liées au contenu d'un SMS
// (encodage GSM-7 / UCS-2, comptage de caractères et de segments).
package sms

// Encoding est l'encodage retenu pour un message.
type Encoding string

const (
	GSM7 Encoding = "gsm7"
	UCS2 Encoding = "ucs2"
)

// Analysis décrit la taille d'un message une fois encodé.
type Analysis struct {
	Encoding   Encoding `json:"encoding"`
	Chars      int      `json:"chars"`      // caractères affichés
	Units      int      `json:"units"`      // unités d'encodage (GSM-7 : extension = 2)
	Segments   int      `json:"segments"`   // nombre de SMS facturés
	PerSegment int      `json:"perSegment"` // capacité d'un segment pour ce message
	Remaining  int      `json:"remaining"`  // unités restantes dans le dernier segment
}

const gsmBasic = "@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?" +
	"¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà"

const gsmExt = "^{}\\[~]|€\f"

var (
	basicSet = toSet(gsmBasic)
	extSet   = toSet(gsmExt)
)

func toSet(s string) map[rune]struct{} {
	m := make(map[rune]struct{}, len(s))
	for _, r := range s {
		m[r] = struct{}{}
	}
	return m
}

// Analyze calcule l'encodage et le nombre de segments d'un message.
func Analyze(msg string) Analysis {
	chars, units := 0, 0
	gsm := true
	for _, r := range msg {
		chars++
		if _, ok := basicSet[r]; ok {
			units++
		} else if _, ok := extSet[r]; ok {
			units += 2
		} else {
			gsm = false
		}
	}
	a := Analysis{Encoding: GSM7, Chars: chars, Units: units}
	single, multi := 160, 153
	if !gsm {
		a.Encoding = UCS2
		single, multi = 70, 67
		// UCS-2 : les caractères hors plan BMP comptent double (paire de substituts).
		a.Units = 0
		for _, r := range msg {
			if r > 0xFFFF {
				a.Units += 2
			} else {
				a.Units++
			}
		}
	}
	switch {
	case a.Units == 0:
		a.PerSegment, a.Remaining = single, single
	case a.Units <= single:
		a.Segments, a.PerSegment = 1, single
		a.Remaining = single - a.Units
	default:
		a.Segments = (a.Units + multi - 1) / multi
		a.PerSegment = multi
		a.Remaining = a.Segments*multi - a.Units
	}
	return a
}
