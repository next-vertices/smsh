package sms

import (
	"regexp"
	"strings"
)

// DefaultCountryCode est l'indicatif appliqué aux numéros nationaux (0612345678).
const DefaultCountryCode = "33"

var (
	splitRe = regexp.MustCompile(`[\n\r,;]+`)
	e164Re  = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)
	spaceRe = regexp.MustCompile(`[\s.\-]`)
	sepRe   = regexp.MustCompile(`[()]`)
)

// NormalizeNumber convertit un numéro saisi librement au format E.164 (+33612345678).
// Accepte : "06 12 34 56 78", "0033 6 12 34 56 78", "+33 (0)6...", "+33612345678".
func NormalizeNumber(raw string) (string, bool) {
	s := strings.Replace(spaceRe.ReplaceAllString(raw, ""), "(0)", "", 1)
	s = sepRe.ReplaceAllString(s, "")
	switch {
	case strings.HasPrefix(s, "+"):
	case strings.HasPrefix(s, "00"):
		s = "+" + s[2:]
	case strings.HasPrefix(s, "0"):
		s = "+" + DefaultCountryCode + s[1:]
	default:
		return raw, false
	}
	return s, e164Re.MatchString(s)
}

// Receivers est le résultat de l'analyse d'une saisie de destinataires.
type Receivers struct {
	Valid   []string `json:"valid"`   // normalisés, dédoublonnés, ordre conservé
	Invalid []string `json:"invalid"` // saisies non reconnues, telles que tapées
	Dupes   int      `json:"dupes"`   // doublons ignorés
}

// ParseReceivers découpe une saisie (séparée par retours à la ligne, virgules ou points-virgules).
func ParseReceivers(text string) Receivers {
	r := Receivers{Valid: []string{}, Invalid: []string{}}
	seen := map[string]bool{}
	for _, part := range splitRe.Split(text, -1) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, ok := NormalizeNumber(part)
		switch {
		case !ok:
			r.Invalid = append(r.Invalid, part)
		case seen[n]:
			r.Dupes++
		default:
			seen[n] = true
			r.Valid = append(r.Valid, n)
		}
	}
	return r
}
