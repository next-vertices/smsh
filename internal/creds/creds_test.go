package creds

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func init() { scryptWorkFactor = 10 } // rapide en test

func validInput() Input {
	return Input{
		Name: "Perso", ServiceName: "sms-ab1-1", Endpoint: "ovh-eu",
		AppKey: "AK-1234567890", AppSecret: "AS-abcdefghijklmnop", ConsumerKey: "CK-zyxwvutsrqponm",
	}
}

func TestMask(t *testing.T) {
	if got := Mask(""); got != "" {
		t.Errorf("empty: %q", got)
	}
	if got := Mask("short"); strings.Contains(got, "sh") || got != "••••••••" {
		t.Errorf("short: %q", got)
	}
	got := Mask("AK-1234567890")
	if !strings.HasPrefix(got, "AK") || !strings.HasSuffix(got, "90") || strings.Contains(got, "1234") {
		t.Errorf("long: %q", got)
	}
}

func TestSaveListDeleteMasked(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, NewMemoryVault())
	acc, err := m.Save(validInput())
	if err != nil {
		t.Fatal(err)
	}
	views, err := m.List()
	if err != nil || len(views) != 1 || !views[0].Active {
		t.Fatalf("views = %+v, %v", views, err)
	}
	v := views[0]
	for _, s := range []string{v.AppKey, v.AppSecret, v.ConsumerKey} {
		if s == "" || strings.Contains(s, "1234567") || strings.Contains(s, "abcdefgh") || strings.Contains(s, "zyxwvut") {
			t.Errorf("secret mal masqué : %q", s)
		}
	}
	// Rien de secret sur disque dans les métadonnées.
	raw, _ := os.ReadFile(filepath.Join(dir, "accounts.json"))
	for _, secret := range []string{"AK-1234567890", "AS-abcdefghijklmnop", "CK-zyxwvutsrqponm"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("secret %q écrit en clair dans accounts.json", secret)
		}
	}
	if err := m.Delete(acc.ID); err != nil {
		t.Fatal(err)
	}
	if st, _ := m.Status(); st.Accounts != 0 {
		t.Errorf("status = %+v", st)
	}
	if _, _, err := m.Active(); !errors.Is(err, ErrNoAccount) {
		t.Errorf("active err = %v", err)
	}
}

func TestUpdateKeepsEmptySecrets(t *testing.T) {
	m := NewManager(t.TempDir(), NewMemoryVault())
	acc, _ := m.Save(validInput())
	in := Input{ID: acc.ID, Name: "Renommé", ServiceName: "sms-ab1-1", Endpoint: "ovh-eu", AppKey: "NEW-KEY-123456789"}
	if _, err := m.Save(in); err != nil {
		t.Fatal(err)
	}
	a, s, err := m.Load(acc.ID)
	if err != nil || a.Name != "Renommé" || s.AppKey != "NEW-KEY-123456789" || s.AppSecret != "AS-abcdefghijklmnop" {
		t.Fatalf("a=%+v s=%+v err=%v", a, s, err)
	}
}

func TestValidation(t *testing.T) {
	m := NewManager(t.TempDir(), NewMemoryVault())
	for name, mut := range map[string]func(*Input){
		"nom":      func(i *Input) { i.Name = " " },
		"service":  func(i *Input) { i.ServiceName = "" },
		"endpoint": func(i *Input) { i.Endpoint = "ovh-xx" },
		"secret":   func(i *Input) { i.AppSecret = "" },
	} {
		in := validInput()
		mut(&in)
		var ve *ValidationError
		if _, err := m.Save(in); !errors.As(err, &ve) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestActiveSwitchOnDelete(t *testing.T) {
	m := NewManager(t.TempDir(), NewMemoryVault())
	a, _ := m.Save(validInput())
	in := validInput()
	in.Name = "Pro"
	b, _ := m.Save(in)
	if act, _, _ := m.Active(); act.ID != a.ID {
		t.Fatal("le premier compte doit être actif")
	}
	_ = m.SetActive(b.ID)
	_ = m.Delete(b.ID)
	if act, _, err := m.Active(); err != nil || act.ID != a.ID {
		t.Fatalf("active = %+v, %v", act, err)
	}
}

func TestAgeVaultRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.age")
	m := NewManager(dir, NewAgeVault(path))

	if _, err := m.Save(validInput()); !errors.Is(err, ErrLocked) {
		t.Fatalf("attendu ErrLocked, got %v", err)
	}
	if err := m.Unlock("short"); !errors.Is(err, ErrWeakPassphase) {
		t.Fatalf("passphrase faible: %v", err)
	}
	if err := m.Unlock("correct horse battery"); err != nil {
		t.Fatal(err)
	}
	acc, err := m.Save(validInput())
	if err != nil {
		t.Fatal(err)
	}
	enc, _ := os.ReadFile(path)
	if strings.Contains(string(enc), "AK-1234567890") {
		t.Fatal("secret en clair dans le fichier age")
	}

	// Nouvelle instance : verrouillée, mauvaise passphrase refusée, bonne acceptée.
	m2 := NewManager(dir, NewAgeVault(path))
	if st, _ := m2.Status(); !st.Locked || !st.Initialized || st.Backend != "age" {
		t.Fatalf("status = %+v", st)
	}
	if err := m2.Unlock("wrong passphrase"); !errors.Is(err, ErrBadPassphrase) {
		t.Fatalf("mauvaise passphrase: %v", err)
	}
	if err := m2.Unlock("correct horse battery"); err != nil {
		t.Fatal(err)
	}
	_, s, err := m2.Load(acc.ID)
	if err != nil || s.AppSecret != "AS-abcdefghijklmnop" {
		t.Fatalf("s=%+v err=%v", s, err)
	}
}
