package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	"smsh/internal/creds"
	"smsh/internal/i18n"
	"smsh/internal/ovh"
	"smsh/internal/service"
	"smsh/internal/store"
)

// boot ouvre le cœur métier (coffre, base locale) comme le fait l'application graphique.
// SMSH_MOCK=1 : simulation locale sans réseau (démonstration et tests).
func boot(ctx context.Context) (*service.Service, func(), error) {
	if os.Getenv("SMSH_MOCK") == "1" {
		return bootMock(ctx)
	}
	dir, err := creds.ConfigDir()
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}
	st, err := store.Open(filepath.Join(dir, "smsh.db"))
	if err != nil {
		return nil, nil, err
	}
	m := creds.Open(dir)
	if err := unlock(m); err != nil {
		st.Close()
		return nil, nil, err
	}
	return service.Open(m, st), func() { st.Close() }, nil
}

func bootMock(ctx context.Context) (*service.Service, func(), error) {
	// SMSH_MOCK_DIR : dossier persistant entre deux appels (démonstration) ; sinon dossier temporaire jetable.
	dir, keep := os.Getenv("SMSH_MOCK_DIR"), true
	if dir == "" {
		var err error
		if dir, err = os.MkdirTemp("", "smsh-cli-mock-"); err != nil {
			return nil, nil, err
		}
		keep = false
	} else if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}
	st, err := store.Open(filepath.Join(dir, "smsh.db"))
	if err != nil {
		return nil, nil, err
	}
	_ = os.Remove(filepath.Join(dir, "accounts.json")) // coffre en mémoire : les secrets ne survivent pas au processus
	m := creds.NewManager(dir, creds.NewMemoryVault())
	if _, err := m.Save(creds.Input{Name: "Démo", ServiceName: "sms-demo-1", Endpoint: "ovh-eu",
		AppKey: "AK-1234567890", AppSecret: "AS-1234567890", ConsumerKey: "CK-1234567890"}); err != nil {
		return nil, nil, err
	}
	mock := ovh.NewMock()
	svc := service.Open(m, st)
	svc.NewClient = func(ovh.Credentials) (ovh.Client, error) { return mock, nil }
	return svc, func() {
		st.Close()
		if !keep {
			os.RemoveAll(dir)
		}
	}, nil
}

// unlock déverrouille un coffre à passphrase : variable SMSH_PASSPHRASE, sinon saisie masquée.
func unlock(m *creds.Manager) error {
	st, err := m.Status()
	if err != nil {
		return err
	}
	if !st.Locked {
		return nil
	}
	if !st.Initialized {
		return failf("%s", i18n.T("cli.vault_not_created"))
	}
	pass := os.Getenv("SMSH_PASSPHRASE")
	if pass == "" {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return failf("%s", i18n.T("cli.vault_locked"))
		}
		fmt.Fprint(os.Stderr, i18n.T("cli.passphrase_prompt"))
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return err
		}
		pass = string(b)
	}
	return m.Unlock(pass)
}

// withService exécute fn avec un contexte annulé par Ctrl-C et le cœur métier ouvert.
func withService(fn func(ctx context.Context, svc *service.Service) error) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	svc, closeFn, err := boot(ctx)
	if err != nil {
		return err
	}
	defer closeFn()
	return fn(ctx, svc)
}

func printJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func interactive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
}

// confirm demande une confirmation (o/N). --yes la remplace ; sans terminal, --yes est obligatoire.
func confirm(question string, yes bool) error {
	if yes {
		return nil
	}
	if !interactive() {
		return usagef("%s", i18n.T("cli.confirm_required"))
	}
	fmt.Fprint(os.Stderr, i18n.T("cli.confirm_prompt", i18n.P{"q": question}))
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "o", "oui", "y", "yes":
		return nil
	}
	return failf("%s", i18n.T("cli.canceled"))
}

func euro(lo, hi float64, cur string) string {
	if cur == "" || cur == "EUR" {
		cur = "€"
	}
	if lo == hi || hi == 0 {
		return fmt.Sprintf("%.3f %s", lo, cur)
	}
	return fmt.Sprintf("%.3f – %.3f %s", lo, hi, cur)
}

func trunc(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if n > 0 && len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func dateFr(t time.Time) string { return t.Local().Format("02/01/2006 15:04:05") }

// readMessage lit le message : --message, --file (« - » = entrée standard).
func readMessage(msg, file string) (string, error) {
	switch {
	case msg != "" && file != "":
		return "", usagef("%s", i18n.T("cli.msg_both"))
	case msg != "":
		return msg, nil
	case file == "":
		return "", usagef("%s", i18n.T("cli.msg_missing"))
	}
	var b []byte
	var err error
	if file == "-" {
		b, err = io.ReadAll(os.Stdin)
	} else {
		b, err = os.ReadFile(file)
	}
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}

// normalizeAt accepte « 2026-11-05 14:30 » en plus des formats du cœur.
func normalizeAt(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 16 && s[10] == ' ' {
		return s[:10] + "T" + s[11:]
	}
	return s
}
