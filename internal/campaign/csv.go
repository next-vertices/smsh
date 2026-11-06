// Package campaign contient la logique des campagnes : import CSV, templates
// avec variables, et moteur d'envoi (limitation de débit, pause/reprise).
package campaign

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"smsh/internal/i18n"
	"smsh/internal/sms"
)

// Table est un CSV chargé : première ligne = en-têtes.
type Table struct {
	Headers []string
	Rows    [][]string
}

// ParseCSV lit un CSV (UTF-8 ou Windows-1252 comme exporté par Excel FR),
// détecte le séparateur (; , tab) et exige une ligne d'en-têtes.
func ParseCSV(data []byte) (Table, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if !utf8.Valid(data) {
		dec, err := charmap.Windows1252.NewDecoder().Bytes(data)
		if err != nil {
			return Table{}, fmt.Errorf("encodage du fichier non reconnu : %w", err)
		}
		data = dec
	}
	first := data
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		first = data[:i]
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = detectDelimiter(first)
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	var recs [][]string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Table{}, fmt.Errorf("CSV invalide : %w", err)
		}
		if len(rec) == 1 && strings.TrimSpace(rec[0]) == "" {
			continue // ligne vide
		}
		recs = append(recs, rec)
	}
	if len(recs) < 2 {
		return Table{}, errors.New(i18n.T("camp.csv_shape"))
	}
	t := Table{Headers: cleanHeaders(recs[0]), Rows: recs[1:]}
	return t, nil
}

func detectDelimiter(line []byte) rune {
	best, bestN := ',', -1
	for _, d := range []rune{';', ',', '\t'} {
		if n := bytes.Count(line, []byte(string(d))); n > bestN {
			best, bestN = d, n
		}
	}
	return best
}

func cleanHeaders(h []string) []string {
	out := make([]string, len(h))
	seen := map[string]bool{}
	for i, v := range h {
		v = strings.TrimSpace(v)
		if v == "" {
			v = fmt.Sprintf("colonne%d", i+1)
		}
		for base, n := v, 2; seen[strings.ToLower(v)]; n++ {
			v = fmt.Sprintf("%s_%d", base, n)
		}
		seen[strings.ToLower(v)] = true
		out[i] = v
	}
	return out
}

var phoneHeaders = []string{"tel", "telephone", "téléphone", "phone", "mobile", "gsm", "portable", "numero", "numéro", "number", "to", "destinataire"}

// DetectPhoneColumn devine la colonne des numéros (-1 si introuvable) : d'abord
// par le nom de l'en-tête, sinon par la colonne dont les valeurs ressemblent le plus à des numéros.
func DetectPhoneColumn(t Table) int {
	for i, h := range t.Headers {
		hl := strings.ToLower(h)
		for _, p := range phoneHeaders {
			if hl == p || strings.Contains(hl, p) {
				return i
			}
		}
	}
	best, bestRatio := -1, 0.8
	for c := range t.Headers {
		ok, n := 0, 0
		for _, row := range t.Rows[:min(len(t.Rows), 50)] {
			if c >= len(row) || strings.TrimSpace(row[c]) == "" {
				continue
			}
			n++
			if _, valid := sms.NormalizeNumber(row[c]); valid {
				ok++
			}
		}
		if n > 0 && float64(ok)/float64(n) >= bestRatio {
			best, bestRatio = c, float64(ok)/float64(n)
		}
	}
	return best
}

// RowIssue décrit une ligne rejetée.
type RowIssue struct {
	Line  int    `json:"line"` // numéro de ligne dans le fichier (en-têtes = 1)
	Value string `json:"value"`
}

// Contact est une ligne valide : numéro normalisé + variables (toutes les colonnes).
type Contact struct {
	Phone string
	Vars  map[string]string
}

// Contacts est le résultat de l'extraction des destinataires d'un CSV.
type Contacts struct {
	Valid   []Contact
	Invalid []RowIssue
	Dupes   int
}

// ExtractContacts normalise la colonne phone et dédoublonne les numéros.
func ExtractContacts(t Table, phoneCol int) (Contacts, error) {
	if phoneCol < 0 || phoneCol >= len(t.Headers) {
		return Contacts{}, errors.New(i18n.T("camp.choose_column"))
	}
	var out Contacts
	seen := map[string]bool{}
	for i, row := range t.Rows {
		raw := ""
		if phoneCol < len(row) {
			raw = strings.TrimSpace(row[phoneCol])
		}
		n, ok := sms.NormalizeNumber(raw)
		switch {
		case !ok:
			out.Invalid = append(out.Invalid, RowIssue{Line: i + 2, Value: raw})
		case seen[n]:
			out.Dupes++
		default:
			seen[n] = true
			vars := make(map[string]string, len(t.Headers))
			for c, h := range t.Headers {
				if c < len(row) {
					vars[h] = strings.TrimSpace(row[c])
				} else {
					vars[h] = ""
				}
			}
			out.Valid = append(out.Valid, Contact{Phone: n, Vars: vars})
		}
	}
	return out, nil
}
