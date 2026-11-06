package i18n

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"fr": FR, "fr-FR": FR, "fr_CA.UTF-8": FR, "FR-be": FR,
		"en": EN, "en-US": EN, "en_GB.UTF-8": EN, "de-DE": EN, "": EN, "C": EN, "frisian": EN,
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, attendu %q", in, got, want)
		}
	}
}

func TestDefaultIsFrench(t *testing.T) {
	if Lang() != FR {
		t.Fatalf("langue par défaut = %q", Lang())
	}
}

func TestTranslateAndParams(t *testing.T) {
	defer SetLang(FR)
	SetLang(FR)
	if got := T("svc.name_required"); got != "Donnez un nom à la campagne." {
		t.Errorf("fr = %q", got)
	}
	SetLang("en-US")
	if got := T("svc.name_required"); got != "Give the campaign a name." {
		t.Errorf("en = %q", got)
	}
	if got := T("svc.insufficient", P{"needed": 5, "available": 2}); got != "Insufficient credits: 5 needed, 2 available." {
		t.Errorf("params = %q", got)
	}
}

func TestPlural(t *testing.T) {
	if got := In(EN, "svc.rows_ignored", P{"count": 1}); got != "1 row ignored: invalid number." {
		t.Errorf("one = %q", got)
	}
	if got := In(EN, "svc.rows_ignored", P{"count": 3}); got != "3 rows ignored: invalid number." {
		t.Errorf("other = %q", got)
	}
	// Le français n'a pas de variantes : la forme de base est utilisée.
	if got := In(FR, "svc.rows_ignored", P{"count": 3}); got != "3 ligne(s) ignorée(s) : numéro invalide." {
		t.Errorf("fr = %q", got)
	}
}

func TestMissingKeyFallsBack(t *testing.T) {
	if got := In(EN, "clé.inexistante"); got != "clé.inexistante" {
		t.Errorf("clé absente = %q", got)
	}
}

var placeholder = regexp.MustCompile(`\{[a-z_]+\}`)

// Clés dont les accolades sont du texte littéral (exemples de variables CSV), propre à chaque langue.
var literalBraces = map[string]bool{"cli.fl_template": true, "cli.camp_long": true, "cli.run_example": true}

func names(s string) string {
	m := placeholder.FindAllString(s, -1)
	sort.Strings(m)
	return strings.Join(m, ",")
}

// Chaque clé existe dans les deux langues (variantes de pluriel comprises) avec les mêmes paramètres.
func TestCatalogsAreConsistent(t *testing.T) {
	base := func(k string) string { return strings.TrimSuffix(strings.TrimSuffix(k, "_one"), "_other") }
	ref := map[string]string{} // clé de base -> paramètres
	for _, lang := range Supported {
		for _, k := range Keys(lang) {
			b := base(k)
			if literalBraces[b] {
				continue
			}
			p := names(catalogs[lang][k])
			if prev, ok := ref[b]; ok && prev != p {
				t.Errorf("%s (%s) : paramètres %q ≠ %q", k, lang, p, prev)
			}
			ref[b] = p
		}
	}
	for _, lang := range Supported {
		have := map[string]bool{}
		for _, k := range Keys(lang) {
			have[base(k)] = true
		}
		for b := range ref {
			if !have[b] {
				t.Errorf("clé %q absente de %s", b, lang)
			}
		}
	}
}
