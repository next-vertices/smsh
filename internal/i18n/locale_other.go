//go:build !windows

package i18n

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// systemLocale lit la locale POSIX (LC_ALL, LC_MESSAGES, LANG), puis, sous macOS, les préférences système.
func systemLocale() string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" && v != "C" && v != "POSIX" {
			return v
		}
	}
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("defaults", "read", "-g", "AppleLocale").Output(); err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}
