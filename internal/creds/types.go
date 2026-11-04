// Package creds gère les comptes OVH et le stockage sécurisé de leurs secrets.
//
// Les secrets (application key/secret, consumer key) vivent uniquement dans un
// Vault : le coffre de l'OS (keyring) ou, à défaut, un fichier chiffré par
// passphrase (age). Le fichier de métadonnées ne contient rien de secret.
package creds

import (
	"errors"
	"strings"
)

var (
	ErrNotFound      = errors.New("compte introuvable")
	ErrNoAccount     = errors.New("aucun compte configuré")
	ErrLocked        = errors.New("coffre verrouillé : passphrase requise")
	ErrBadPassphrase = errors.New("passphrase incorrecte")
	ErrWeakPassphase = errors.New("la passphrase doit contenir au moins 8 caractères")
)

// ValidationError signale une saisie invalide (message affichable tel quel).
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// Secrets sont les valeurs sensibles d'un compte. Ne jamais les journaliser
// ni les renvoyer au front.
type Secrets struct {
	AppKey      string `json:"appKey"`
	AppSecret   string `json:"appSecret"`
	ConsumerKey string `json:"consumerKey"`
}

// Account est la partie non secrète d'un compte (persistée en clair).
type Account struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ServiceName string `json:"serviceName"`
	Endpoint    string `json:"endpoint"`
}

// Input est une saisie de formulaire. Pour un compte existant (ID renseigné),
// un secret vide signifie « conserver la valeur actuelle ».
type Input struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ServiceName string `json:"serviceName"`
	Endpoint    string `json:"endpoint"`
	AppKey      string `json:"appKey"`
	AppSecret   string `json:"appSecret"`
	ConsumerKey string `json:"consumerKey"`
}

// View est ce que le front a le droit de voir : secrets masqués.
type View struct {
	Account
	AppKey      string `json:"appKey"`
	AppSecret   string `json:"appSecret"`
	ConsumerKey string `json:"consumerKey"`
	Active      bool   `json:"active"`
}

// Mask masque un secret en ne laissant apparaître que ses extrémités
// quand il est assez long pour que ce soit sans risque.
func Mask(s string) string {
	if s == "" {
		return ""
	}
	if len(s) < 12 {
		return strings.Repeat("•", 8)
	}
	return s[:2] + strings.Repeat("•", 8) + s[len(s)-2:]
}

// Vault stocke les secrets par identifiant de compte.
type Vault interface {
	// Name identifie le backend ("keyring", "age", "memory").
	Name() string
	Get(id string) (Secrets, error) // ErrNotFound si absent
	Set(id string, s Secrets) error
	Delete(id string) error
}

// Lockable est implémenté par les coffres protégés par passphrase.
type Lockable interface {
	Locked() bool
	// Initialized indique si le coffre existe déjà sur disque.
	Initialized() bool
	// Unlock ouvre le coffre ; s'il n'existe pas encore, le crée avec cette passphrase.
	Unlock(passphrase string) error
}
