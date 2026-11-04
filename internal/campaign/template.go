package campaign

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var varRe = regexp.MustCompile(`\{([^{}]+)\}`)

// Variables liste les variables {nom} d'un template, sans doublon, dans l'ordre d'apparition.
func Variables(tpl string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range varRe.FindAllStringSubmatch(tpl, -1) {
		name := strings.TrimSpace(m[1])
		if !seen[strings.ToLower(name)] {
			seen[strings.ToLower(name)] = true
			out = append(out, name)
		}
	}
	return out
}

// Unknown renvoie les variables du template absentes des en-têtes (insensible à la casse).
func Unknown(tpl string, headers []string) []string {
	known := map[string]bool{}
	for _, h := range headers {
		known[strings.ToLower(h)] = true
	}
	var out []string
	for _, v := range Variables(tpl) {
		if !known[strings.ToLower(v)] {
			out = append(out, v)
		}
	}
	return out
}

// Render remplace les {variables} par les valeurs d'un contact.
// Une variable inconnue est une erreur (jamais de « {prenom} » envoyé tel quel).
// Une valeur vide est remplacée par une chaîne vide.
func Render(tpl string, vars map[string]string) (string, error) {
	lower := make(map[string]string, len(vars))
	for k, v := range vars {
		lower[strings.ToLower(k)] = v
	}
	var missing []string
	out := varRe.ReplaceAllStringFunc(tpl, func(m string) string {
		name := strings.ToLower(strings.TrimSpace(m[1 : len(m)-1]))
		v, ok := lower[name]
		if !ok {
			missing = append(missing, m)
		}
		return v
	})
	if len(missing) > 0 {
		sort.Strings(missing)
		return "", fmt.Errorf("variable(s) inconnue(s) : %s", strings.Join(missing, ", "))
	}
	return out, nil
}
