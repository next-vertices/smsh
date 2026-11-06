package creds

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"smsh/internal/i18n"
	"smsh/internal/ovh"
)

// ConfigDir renvoie le dossier de configuration de smsh (partagé GUI/CLI).
func ConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "smsh"), nil
}

// Status décrit l'état du stockage, pour l'UI.
type Status struct {
	Backend     string `json:"backend"`     // "keyring" | "age" | "memory"
	Locked      bool   `json:"locked"`      // true : passphrase à fournir
	Initialized bool   `json:"initialized"` // coffre à passphrase déjà créé
	Accounts    int    `json:"accounts"`
}

type meta struct {
	Active   string    `json:"active"`
	Accounts []Account `json:"accounts"`
}

// Manager gère la liste des comptes (métadonnées) et leurs secrets (Vault).
type Manager struct {
	dir   string
	vault Vault

	mu sync.Mutex
}

// NewManager crée un Manager avec un coffre explicite.
func NewManager(dir string, vault Vault) *Manager { return &Manager{dir: dir, vault: vault} }

// Open choisit le meilleur coffre disponible : coffre de l'OS, sinon fichier age.
// SMSH_VAULT=age|keyring force le choix.
func Open(dir string) *Manager {
	agePath := filepath.Join(dir, "secrets.age")
	switch os.Getenv("SMSH_VAULT") {
	case "age":
		return NewManager(dir, NewAgeVault(agePath))
	case "keyring":
		return NewManager(dir, NewKeyringVault())
	}
	if KeyringAvailable() {
		return NewManager(dir, NewKeyringVault())
	}
	return NewManager(dir, NewAgeVault(agePath))
}

func (m *Manager) metaPath() string { return filepath.Join(m.dir, "accounts.json") }

func (m *Manager) readMeta() (meta, error) {
	var md meta
	raw, err := os.ReadFile(m.metaPath())
	if errors.Is(err, os.ErrNotExist) {
		return md, nil
	}
	if err != nil {
		return md, err
	}
	return md, json.Unmarshal(raw, &md)
}

func (m *Manager) writeMeta(md meta) error {
	raw, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(m.metaPath(), raw)
}

// Status renvoie l'état du stockage.
func (m *Manager) Status() (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	md, err := m.readMeta()
	if err != nil {
		return Status{}, err
	}
	st := Status{Backend: m.vault.Name(), Accounts: len(md.Accounts)}
	if l, ok := m.vault.(Lockable); ok {
		st.Locked, st.Initialized = l.Locked(), l.Initialized()
	}
	return st, nil
}

// Unlock ouvre (ou crée) le coffre à passphrase. Sans effet pour le keyring.
func (m *Manager) Unlock(passphrase string) error {
	if l, ok := m.vault.(Lockable); ok {
		return l.Unlock(passphrase)
	}
	return nil
}

func (m *Manager) checkUnlocked() error {
	if l, ok := m.vault.(Lockable); ok && l.Locked() {
		return ErrLocked
	}
	return nil
}

// List renvoie les comptes avec secrets masqués.
func (m *Manager) List() ([]View, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkUnlocked(); err != nil {
		return nil, err
	}
	md, err := m.readMeta()
	if err != nil {
		return nil, err
	}
	views := make([]View, 0, len(md.Accounts))
	for _, a := range md.Accounts {
		v := View{Account: a, Active: a.ID == md.Active}
		if s, err := m.vault.Get(a.ID); err == nil {
			v.AppKey, v.AppSecret, v.ConsumerKey = Mask(s.AppKey), Mask(s.AppSecret), Mask(s.ConsumerKey)
		} else if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		views = append(views, v)
	}
	return views, nil
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func validate(in *Input, existing bool) error {
	in.Name = strings.TrimSpace(in.Name)
	in.ServiceName = strings.TrimSpace(in.ServiceName)
	in.Endpoint = strings.TrimSpace(in.Endpoint)
	in.AppKey = strings.TrimSpace(in.AppKey)
	in.AppSecret = strings.TrimSpace(in.AppSecret)
	in.ConsumerKey = strings.TrimSpace(in.ConsumerKey)
	if in.Name == "" {
		return &ValidationError{"Le nom du compte est requis."}
	}
	if in.ServiceName == "" {
		return &ValidationError{"Le serviceName est requis (ex. sms-ab123456-1)."}
	}
	if !slices.Contains(ovh.Endpoints, in.Endpoint) {
		return &ValidationError{"Endpoint OVH inconnu."}
	}
	if !existing && (in.AppKey == "" || in.AppSecret == "" || in.ConsumerKey == "") {
		return &ValidationError{"Application key, application secret et consumer key sont requis."}
	}
	return nil
}

// Resolve fusionne une saisie avec les secrets existants (champs vides conservés)
// sans rien persister : sert à tester une configuration avant de l'enregistrer.
func (m *Manager) Resolve(in Input) (Account, Secrets, error) {
	existing := in.ID != ""
	if err := validate(&in, existing); err != nil {
		return Account{}, Secrets{}, err
	}
	acc := Account{ID: in.ID, Name: in.Name, ServiceName: in.ServiceName, Endpoint: in.Endpoint}
	sec := Secrets{AppKey: in.AppKey, AppSecret: in.AppSecret, ConsumerKey: in.ConsumerKey}
	if existing {
		cur, err := m.vault.Get(in.ID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return Account{}, Secrets{}, err
		}
		if sec.AppKey == "" {
			sec.AppKey = cur.AppKey
		}
		if sec.AppSecret == "" {
			sec.AppSecret = cur.AppSecret
		}
		if sec.ConsumerKey == "" {
			sec.ConsumerKey = cur.ConsumerKey
		}
		if sec.AppKey == "" || sec.AppSecret == "" || sec.ConsumerKey == "" {
			return Account{}, Secrets{}, &ValidationError{i18n.T("creds.secrets_missing")}
		}
	}
	return acc, sec, nil
}

// Save crée ou met à jour un compte. Le premier compte devient actif.
func (m *Manager) Save(in Input) (Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkUnlocked(); err != nil {
		return Account{}, err
	}
	acc, sec, err := m.Resolve(in)
	if err != nil {
		return Account{}, err
	}
	md, err := m.readMeta()
	if err != nil {
		return Account{}, err
	}
	if acc.ID == "" {
		acc.ID = newID()
		md.Accounts = append(md.Accounts, acc)
	} else {
		i := slices.IndexFunc(md.Accounts, func(a Account) bool { return a.ID == acc.ID })
		if i < 0 {
			return Account{}, ErrNotFound
		}
		md.Accounts[i] = acc
	}
	if md.Active == "" {
		md.Active = acc.ID
	}
	// Secrets d'abord : en cas d'échec, aucune métadonnée orpheline.
	if err := m.vault.Set(acc.ID, sec); err != nil {
		return Account{}, err
	}
	if err := m.writeMeta(md); err != nil {
		return Account{}, err
	}
	return acc, nil
}

// Delete supprime un compte et ses secrets.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkUnlocked(); err != nil {
		return err
	}
	md, err := m.readMeta()
	if err != nil {
		return err
	}
	i := slices.IndexFunc(md.Accounts, func(a Account) bool { return a.ID == id })
	if i < 0 {
		return ErrNotFound
	}
	if err := m.vault.Delete(id); err != nil {
		return err
	}
	md.Accounts = slices.Delete(md.Accounts, i, i+1)
	if md.Active == id {
		md.Active = ""
		if len(md.Accounts) > 0 {
			md.Active = md.Accounts[0].ID
		}
	}
	return m.writeMeta(md)
}

// SetActive choisit le compte utilisé par défaut (GUI et CLI).
func (m *Manager) SetActive(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	md, err := m.readMeta()
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(md.Accounts, func(a Account) bool { return a.ID == id }) {
		return ErrNotFound
	}
	md.Active = id
	return m.writeMeta(md)
}

// Load renvoie un compte et ses secrets en clair (usage interne uniquement).
func (m *Manager) Load(id string) (Account, Secrets, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.load(id)
}

func (m *Manager) load(id string) (Account, Secrets, error) {
	if err := m.checkUnlocked(); err != nil {
		return Account{}, Secrets{}, err
	}
	md, err := m.readMeta()
	if err != nil {
		return Account{}, Secrets{}, err
	}
	i := slices.IndexFunc(md.Accounts, func(a Account) bool { return a.ID == id })
	if i < 0 {
		return Account{}, Secrets{}, ErrNotFound
	}
	sec, err := m.vault.Get(id)
	return md.Accounts[i], sec, err
}

// Active renvoie le compte actif et ses secrets (ErrNoAccount si aucun).
func (m *Manager) Active() (Account, Secrets, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkUnlocked(); err != nil {
		return Account{}, Secrets{}, err
	}
	md, err := m.readMeta()
	if err != nil {
		return Account{}, Secrets{}, err
	}
	if md.Active == "" {
		return Account{}, Secrets{}, ErrNoAccount
	}
	return m.load(md.Active)
}
