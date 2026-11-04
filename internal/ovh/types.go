// Package ovh encapsule l'API SMS d'OVH derrière une interface testable.
package ovh

import (
	"context"
	"time"
)

// Endpoints supportés (noms go-ovh).
var Endpoints = []string{"ovh-eu", "ovh-ca", "ovh-us"}

// Credentials sont les clés d'application OVH. Ne jamais les journaliser.
type Credentials struct {
	Endpoint    string
	AppKey      string
	AppSecret   string
	ConsumerKey string
}

// AccountInfo est l'état d'un service SMS (GET /sms/{serviceName}).
type AccountInfo struct {
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	CreditsLeft float64 `json:"creditsLeft"`
}

// Sender est un expéditeur déclaré sur le service.
type Sender struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "enable" pour un expéditeur utilisable
}

// SendRequest est une demande d'envoi (POST /sms/{serviceName}/jobs).
// Les destinataires doivent déjà être normalisés au format international.
type SendRequest struct {
	Service    string
	Message    string
	Receivers  []string
	Sender     string     // vide : expéditeur par défaut (senderForResponse)
	SendAt     *time.Time // nil : envoi immédiat
	StopClause bool       // true : OVH ajoute la mention STOP (SMS commercial)
	Unicode    bool       // true : message hors GSM-7
}

// SendResult est la réponse d'OVH à un envoi.
type SendResult struct {
	IDs              []int64  `json:"ids"`
	ValidReceivers   []string `json:"validReceivers"`
	InvalidReceivers []string `json:"invalidReceivers"`
	CreditsRemoved   float64  `json:"totalCreditsRemoved"`
}

// Client est l'accès à l'API SMS d'OVH.
type Client interface {
	ListServices(ctx context.Context) ([]string, error)
	Account(ctx context.Context, service string) (AccountInfo, error)
	Senders(ctx context.Context, service string) ([]Sender, error)
	Send(ctx context.Context, req SendRequest) (SendResult, error)
}
