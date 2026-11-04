package sms

import "strings"

// dialCodes associe un indicatif international (sans "+") au code pays OVH
// (sms.CountryEnum, minuscule). Table volontairement limitée aux destinations
// courantes ; un indicatif absent donne un pays inconnu (tarif non calculé).
// Indicatifs partagés (+1) : attribués au pays le plus probable (États-Unis).
var dialCodes = map[string]string{
	"1": "us", "7": "ru", "20": "eg", "27": "za", "30": "gr", "31": "nl", "32": "be", "33": "fr",
	"34": "es", "39": "it", "40": "ro", "41": "ch", "43": "at", "44": "gb", "45": "dk", "46": "se",
	"47": "no", "48": "pl", "49": "de", "51": "pe", "52": "mx", "54": "ar", "55": "br", "56": "cl",
	"57": "co", "58": "ve", "60": "my", "61": "au", "62": "id", "63": "ph", "64": "nz", "65": "sg",
	"66": "th", "81": "jp", "82": "kr", "84": "vn", "86": "cn", "90": "tr", "91": "in", "92": "pk",
	"94": "lk", "98": "ir",
	"212": "ma", "213": "dz", "216": "tn", "221": "sn", "225": "ci", "237": "cm", "243": "cd",
	"262": "re", "269": "yt", "351": "pt", "352": "lu", "353": "ie", "354": "is", "355": "al",
	"356": "mt", "357": "cy", "358": "fi", "359": "bg", "36": "hu", "370": "lt", "371": "lv",
	"372": "ee", "373": "md", "374": "am", "375": "by", "376": "ad", "378": "sm", "380": "ua",
	"381": "rs", "382": "me", "385": "hr", "386": "si", "387": "ba", "389": "mk", "420": "cz",
	"421": "sk", "423": "li", "590": "gp", "594": "gf", "596": "mq", "687": "nc", "689": "pf",
	"852": "hk", "886": "tw", "961": "lb", "962": "jo", "966": "sa", "971": "ae", "972": "il",
	"974": "qa",
}

// CountryOf renvoie le code pays OVH d'un numéro E.164 (plus long préfixe), "" si inconnu.
func CountryOf(e164 string) string {
	digits := strings.TrimPrefix(e164, "+")
	for n := 3; n >= 1; n-- {
		if len(digits) < n {
			continue
		}
		if c, ok := dialCodes[digits[:n]]; ok {
			return c
		}
	}
	return ""
}
