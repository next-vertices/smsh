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
	Type   string `json:"type"`   // alpha, numeric, shortcode, virtual…
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

// MessageEstimate est l'analyse d'un message par OVH (POST /sms/estimate).
type MessageEstimate struct {
	Characters int    `json:"characters"`
	Encoding   string `json:"encoding"` // "7bits" | "unicode"
	PerPart    int    `json:"perPart"`
	Parts      int    `json:"parts"` // nombre de SMS (segments) facturés par destinataire
}

// DestinationRate est le tarif d'un SMS vers un pays (GET /sms/rates/destinations).
type DestinationRate struct {
	Country       string  `json:"country"`
	CreditsPerSMS float64 `json:"creditsPerSms"`
	PricePerSMS   float64 `json:"pricePerSms"` // prix public HT
	Currency      string  `json:"currency"`
}

// PackTier est un palier de prix selon la taille du pack (GET /sms/rates/packs).
type PackTier struct {
	CreditsMin  int64   `json:"creditsMin"`
	PricePerSMS float64 `json:"pricePerSms"`
}

// Client est l'accès à l'API SMS d'OVH.
type Client interface {
	ListServices(ctx context.Context) ([]string, error)
	Account(ctx context.Context, service string) (AccountInfo, error)
	Senders(ctx context.Context, service string) ([]Sender, error)
	Send(ctx context.Context, req SendRequest) (SendResult, error)
	// EstimateMessage fait analyser le message par OVH (encodage, segments).
	EstimateMessage(ctx context.Context, message string, stopClause bool, senderType string) (MessageEstimate, error)
	// DestinationRate donne le tarif d'un SMS vers un pays (code ISO minuscule).
	DestinationRate(ctx context.Context, billingCountry, country string) (DestinationRate, error)
	// PackTiers donne les paliers de prix des packs de crédits pour ce pays.
	PackTiers(ctx context.Context, billingCountry, country string) ([]PackTier, error)
}
