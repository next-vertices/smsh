//go:build windows

package i18n

import "syscall"

// systemLocale renvoie la langue d'affichage de Windows (et non le format régional) :
// « fr » si la langue principale est le français, « en » sinon.
func systemLocale() string {
	k32 := syscall.NewLazyDLL("kernel32.dll")
	r, _, _ := k32.NewProc("GetUserDefaultUILanguage").Call()
	if r == 0 {
		return ""
	}
	const primaryLangMask, langFrench = 0x3FF, 0x0C
	if uint16(r)&primaryLangMask == langFrench {
		return "fr-FR"
	}
	return "en-GB"
}
