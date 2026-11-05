//go:build windows

package main

import (
	"os"
	"syscall"
)

// attachConsole rattache le processus (sous-système « fenêtre ») à la console du terminal
// qui l'a lancé, pour que la CLI puisse lire et écrire. Les flux déjà redirigés
// (pipe, fichier) sont conservés tels quels.
func attachConsole() {
	k32 := syscall.NewLazyDLL("kernel32.dll")
	const attachParentProcess = ^uintptr(0)
	if r, _, _ := k32.NewProc("AttachConsole").Call(attachParentProcess); r == 0 {
		return // pas de console parente (lancé depuis l'Explorateur, par exemple)
	}
	reopen := func(f **os.File, std int, name string, flag int) {
		if h, err := syscall.GetStdHandle(std); err == nil && h != 0 && h != syscall.InvalidHandle {
			return // déjà valide (redirigé)
		}
		if nf, err := os.OpenFile(name, flag, 0); err == nil {
			*f = nf
		}
	}
	reopen(&os.Stdin, syscall.STD_INPUT_HANDLE, "CONIN$", os.O_RDWR)
	reopen(&os.Stdout, syscall.STD_OUTPUT_HANDLE, "CONOUT$", os.O_RDWR)
	reopen(&os.Stderr, syscall.STD_ERROR_HANDLE, "CONOUT$", os.O_RDWR)
}
