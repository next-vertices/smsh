package creds

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"filippo.io/age"
	"github.com/zalando/go-keyring"
)

// ---------- mémoire (tests) ----------

type memVault struct {
	mu sync.Mutex
	m  map[string]Secrets
}

// NewMemoryVault renvoie un coffre en mémoire (tests, mode démo).
func NewMemoryVault() Vault { return &memVault{m: map[string]Secrets{}} }

func (v *memVault) Name() string { return "memory" }
func (v *memVault) Get(id string) (Secrets, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	s, ok := v.m[id]
	if !ok {
		return Secrets{}, ErrNotFound
	}
	return s, nil
}
func (v *memVault) Set(id string, s Secrets) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.m[id] = s
	return nil
}
func (v *memVault) Delete(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.m, id)
	return nil
}

// ---------- coffre de l'OS ----------

const keyringService = "smsh"

type keyringVault struct{}

// NewKeyringVault utilise le coffre de l'OS (Credential Manager, Keychain, Secret Service).
func NewKeyringVault() Vault { return keyringVault{} }

func (keyringVault) Name() string { return "keyring" }

func keyringUser(id string) string { return "account/" + id }

func (keyringVault) Get(id string) (Secrets, error) {
	raw, err := keyring.Get(keyringService, keyringUser(id))
	if errors.Is(err, keyring.ErrNotFound) {
		return Secrets{}, ErrNotFound
	}
	if err != nil {
		return Secrets{}, err
	}
	var s Secrets
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return Secrets{}, err
	}
	return s, nil
}

func (keyringVault) Set(id string, s Secrets) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return keyring.Set(keyringService, keyringUser(id), string(raw))
}

func (keyringVault) Delete(id string) error {
	err := keyring.Delete(keyringService, keyringUser(id))
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// KeyringAvailable teste réellement l'écriture/lecture dans le coffre de l'OS.
func KeyringAvailable() bool {
	const probe = "probe"
	if err := keyring.Set(keyringService, probe, "ok"); err != nil {
		return false
	}
	v, err := keyring.Get(keyringService, probe)
	_ = keyring.Delete(keyringService, probe)
	return err == nil && v == "ok"
}

// ---------- fichier chiffré par passphrase (age) ----------

// scryptWorkFactor : 0 = défaut d'age (~1 s). Abaissé dans les tests.
var scryptWorkFactor = 0

type ageVault struct {
	path string

	mu   sync.Mutex
	pass string // vide = verrouillé
	data map[string]Secrets
}

// NewAgeVault crée un coffre chiffré par passphrase stocké dans path.
func NewAgeVault(path string) Vault { return &ageVault{path: path} }

func (v *ageVault) Name() string { return "age" }

func (v *ageVault) Locked() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.pass == ""
}

func (v *ageVault) Initialized() bool {
	_, err := os.Stat(v.path)
	return err == nil
}

func (v *ageVault) Unlock(pass string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.Initialized() {
		if len(pass) < 8 {
			return ErrWeakPassphase
		}
		v.pass, v.data = pass, map[string]Secrets{}
		if err := v.flush(); err != nil {
			v.pass, v.data = "", nil
			return err
		}
		return nil
	}
	enc, err := os.ReadFile(v.path)
	if err != nil {
		return err
	}
	id, err := age.NewScryptIdentity(pass)
	if err != nil {
		return err
	}
	r, err := age.Decrypt(bytes.NewReader(enc), id)
	if err != nil {
		var noMatch *age.NoIdentityMatchError
		if errors.As(err, &noMatch) {
			return ErrBadPassphrase
		}
		return err
	}
	plain, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	data := map[string]Secrets{}
	if err := json.Unmarshal(plain, &data); err != nil {
		return err
	}
	v.pass, v.data = pass, data
	return nil
}

func (v *ageVault) flush() error {
	plain, err := json.Marshal(v.data)
	if err != nil {
		return err
	}
	rcpt, err := age.NewScryptRecipient(v.pass)
	if err != nil {
		return err
	}
	if scryptWorkFactor > 0 {
		rcpt.SetWorkFactor(scryptWorkFactor)
	}
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, rcpt)
	if err != nil {
		return err
	}
	if _, err := w.Write(plain); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return writeFileAtomic(v.path, buf.Bytes())
}

func (v *ageVault) Get(id string) (Secrets, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.pass == "" {
		return Secrets{}, ErrLocked
	}
	s, ok := v.data[id]
	if !ok {
		return Secrets{}, ErrNotFound
	}
	return s, nil
}

func (v *ageVault) Set(id string, s Secrets) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.pass == "" {
		return ErrLocked
	}
	v.data[id] = s
	return v.flush()
}

func (v *ageVault) Delete(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.pass == "" {
		return ErrLocked
	}
	delete(v.data, id)
	return v.flush()
}

// writeFileAtomic écrit via un fichier temporaire puis renomme (droits 0600).
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := os.Chmod(tmp.Name(), 0o600); err != nil && !os.IsNotExist(err) {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
