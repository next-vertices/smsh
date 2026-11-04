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

// BatchRequest crée une campagne native OVH (POST /sms/{serviceName}/batches).
// Le même message est envoyé à tous les destinataires.
type BatchRequest struct {
	Service    string
	Name       string
	Message    string
	Sender     string // vide : expéditeur par défaut (senderForResponse)
	To         []string
	SendAt     *time.Time
	StopClause bool
	Tag        string
}

// BatchError est une erreur d'un destinataire dans un batch.
type BatchError struct {
	Receiver string `json:"receiver"`
	Message  string `json:"message"`
}

// Batch est l'état d'une campagne OVH.
type Batch struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	Status           string       `json:"status"` // PENDING, INSERTING, INSERTED, COMPLETED, FAILED, TO_CANCEL, CANCELING, CANCELED
	TotalRecords     int64        `json:"totalRecords"`
	ProcessedRecords int64        `json:"processedRecords"`
	EstimatedCredits float64      `json:"estimatedCredits"`
	Errors           []BatchError `json:"errors"`
}

// BatchStats est la ventilation des SMS d'un batch.
type BatchStats struct {
	Credits          float64 `json:"credits"`
	EstimatedCredits float64 `json:"estimatedCredits"`
	Delivered        int64   `json:"delivered"`
	Failed           int64   `json:"failed"`
	Pending          int64   `json:"pending"`
	Sent             int64   `json:"sent"`
	Stoplisted       int64   `json:"stoplisted"`
}

// Client est l'accès à l'API SMS d'OVH.
type Client interface {
	ListServices(ctx context.Context) ([]string, error)
	Account(ctx context.Context, service string) (AccountInfo, error)
	Senders(ctx context.Context, service string) ([]Sender, error)
	Send(ctx context.Context, req SendRequest) (SendResult, error)
	// Campagnes natives OVH (batches).
	CreateBatch(ctx context.Context, req BatchRequest) (Batch, error)
	GetBatch(ctx context.Context, service, id string) (Batch, error)
	BatchStats(ctx context.Context, service, id string) (BatchStats, error)
	CancelBatch(ctx context.Context, service, id string) (Batch, error)
	// EstimateMessage fait analyser le message par OVH (encodage, segments).
	EstimateMessage(ctx context.Context, message string, stopClause bool, senderType string) (MessageEstimate, error)
	// DestinationRate donne le tarif d'un SMS vers un pays (code ISO minuscule).
	DestinationRate(ctx context.Context, billingCountry, country string) (DestinationRate, error)
	// PackTiers donne les paliers de prix des packs de crédits pour ce pays.
	PackTiers(ctx context.Context, billingCountry, country string) ([]PackTier, error)
}
