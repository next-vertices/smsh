// Package cli : interface en ligne de commande, qui partage le coffre de clés,
// la base locale et tout le cœur métier avec l'application graphique.
// Elle est embarquée dans le même exécutable que l'application (smsh.exe <commande>).
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"smsh/internal/i18n"
	"smsh/internal/service"
)

// Version est renseignée à la compilation : -ldflags "-X smsh/internal/cli.Version=1.0.0".
var Version = "dev"

// Codes de sortie : 0 succès, 1 erreur d'exécution, 2 erreur d'usage.
const (
	exitErr   = 1
	exitUsage = 2
)

// usageError signale une erreur de syntaxe de commande (code de sortie 2).
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

// plainError : erreur produite par la CLI elle-même, affichée telle quelle.
type plainError struct{ msg string }

func (e plainError) Error() string { return e.msg }

func failf(format string, args ...any) error { return plainError{fmt.Sprintf(format, args...)} }

func usagef(format string, args ...any) error { return usageError{fmt.Sprintf(format, args...)} }

var asJSON bool

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "smsh",
		Short:         i18n.T("cli.root_short"),
		Long:          i18n.T("cli.root_long"),
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolVar(&asJSON, "json", false, i18n.T("cli.flag_json"))
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err.Error()} })
	root.AddCommand(statusCmd(), accountCmd(), sendCmd(), lsCmd(), rmCmd(), syncCmd(), campaignCmd())
	return root
}

// IsCLI indique si les arguments (sans le nom du programme) demandent la ligne de commande
// plutôt que la fenêtre : première valeur = sous-commande ou option connue.
func IsCLI(args []string) bool {
	if len(args) == 0 {
		return false
	}
	root := newRoot()
	for _, c := range root.Commands() {
		if c.Name() == args[0] {
			return true
		}
	}
	switch args[0] {
	case "help", "completion", "--help", "-h", "--version", "-v", "--json":
		return true
	}
	return false
}

// Run exécute la CLI et renvoie le code de sortie.
func Run(args []string) int {
	i18n.SetLang(i18n.Detect())
	root := newRoot()
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return 0
	}
	var ue usageError
	code := exitErr
	if errors.As(err, &ue) {
		code = exitUsage
	}
	var pe plainError
	var p service.Problem
	switch {
	case errors.As(err, &ue):
		p = service.Problem{Kind: "usage", Message: ue.msg}
	case errors.As(err, &pe):
		p = service.Problem{Kind: "cli", Message: pe.msg}
	case errors.Is(err, context.Canceled):
		p = service.Problem{Kind: "cli", Message: i18n.T("cli.interrupted")}
	default:
		p = service.Describe(err)
	}
	if asJSON {
		printJSON(os.Stdout, map[string]any{"error": p})
	} else {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error_prefix"), p.Message)
		if code == exitUsage {
			fmt.Fprintln(os.Stderr, i18n.T("cli.help_hint"))
		}
	}
	return code
}

// exactly valide le nombre d'arguments positionnels (erreur d'usage, code 2).
func exactly(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return usagef("%s", i18n.T("cli.args_count", i18n.P{"cmd": cmd.CommandPath(), "n": n, "got": len(args)}))
		}
		return nil
	}
}
