// Package i18n fournit les traductions des messages du cœur (erreurs, avertissements, CLI).
//
// Les catalogues sont des fichiers JSON plats (locales/<langue>.json), embarqués dans le binaire.
// Paramètres nommés {nom} ; pluriels par clés suffixées _one / _other avec le paramètre "count".
// Langue par défaut : fr-FR. Si la langue du système n'est pas française : en-GB.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

const (
	FR = "fr-FR"
	EN = "en-GB"
)

// Langues prises en charge (ordre d'affichage).
var Supported = []string{FR, EN}

//go:embed locales/*.json
var files embed.FS

var catalogs = map[string]map[string]string{}

func init() {
	for _, lang := range Supported {
		b, err := files.ReadFile("locales/" + lang + ".json")
		if err != nil {
			panic("i18n: catalogue manquant : " + lang)
		}
		m := map[string]string{}
		if err := json.Unmarshal(b, &m); err != nil {
			panic("i18n: catalogue invalide " + lang + " : " + err.Error())
		}
		catalogs[lang] = m
	}
	cur.Store(FR)
}

var cur atomic.Value // string

// Normalize ramène un identifiant de langue (fr, fr_CA.UTF-8, en-US…) à une langue prise en charge :
// tout ce qui commence par « fr » donne fr-FR, le reste en-GB.
func Normalize(tag string) string {
	t := strings.ToLower(strings.TrimSpace(tag))
	if t == "fr" || strings.HasPrefix(t, "fr-") || strings.HasPrefix(t, "fr_") {
		return FR
	}
	return EN
}

// SetLang change la langue courante et renvoie la langue effectivement appliquée.
func SetLang(tag string) string {
	l := Normalize(tag)
	cur.Store(l)
	return l
}

// Lang renvoie la langue courante.
func Lang() string { return cur.Load().(string) }

// Detect renvoie la langue du système : variable SMSH_LANG, puis la locale de l'utilisateur.
func Detect() string {
	if v := os.Getenv("SMSH_LANG"); v != "" {
		return Normalize(v)
	}
	return Normalize(systemLocale())
}

// P regroupe les paramètres nommés d'un message.
type P map[string]any

// T traduit une clé dans la langue courante.
func T(key string, params ...P) string { return In(Lang(), key, params...) }

// In traduit une clé dans une langue donnée. Une clé absente de la langue retombe sur fr-FR,
// puis sur la clé elle-même (visible, donc facile à repérer).
func In(lang, key string, params ...P) string {
	var p P
	if len(params) > 0 {
		p = params[0]
	}
	msg, ok := lookup(lang, key, p)
	if !ok {
		if msg, ok = lookup(FR, key, p); !ok {
			return key
		}
	}
	for k, v := range p {
		msg = strings.ReplaceAll(msg, "{"+k+"}", fmt.Sprint(v))
	}
	return msg
}

func lookup(lang, key string, p P) (string, bool) {
	cat := catalogs[Normalize(lang)]
	if n, ok := count(p); ok {
		suffix := "_other"
		if isOne(Normalize(lang), n) {
			suffix = "_one"
		}
		if m, ok := cat[key+suffix]; ok {
			return m, true
		}
	}
	m, ok := cat[key]
	return m, ok
}

func count(p P) (float64, bool) {
	switch n := p["count"].(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

// isOne : forme singulière. Français : 0 et 1 ; anglais : 1.
func isOne(lang string, n float64) bool {
	if lang == FR {
		return n >= 0 && n < 2
	}
	return n == 1
}

// Keys liste les clés d'un catalogue (tests de cohérence).
func Keys(lang string) []string {
	out := make([]string, 0, len(catalogs[lang]))
	for k := range catalogs[lang] {
		out = append(out, k)
	}
	return out
}
