package ovh

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	goovh "github.com/ovh/go-ovh/ovh"
)

// Kind classe une erreur pour que l'UI puisse l'expliquer clairement.
type Kind string

const (
	KindCredentials         Kind = "credentials"
	KindRateLimit           Kind = "rate_limit"
	KindInsufficientCredits Kind = "insufficient_credits"
	KindInvalidReceiver     Kind = "invalid_receiver"
	KindInvalidRequest      Kind = "invalid_request"
	KindNotFound            Kind = "not_found"
	KindNetwork             Kind = "network"
	KindUnknown             Kind = "unknown"
)

// Error est l'erreur typée renvoyée par le Client.
type Error struct {
	Kind    Kind
	Message string // message lisible en français
	Status  int    // code HTTP, 0 si inconnu
	Err     error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Err }

// KindOf renvoie la catégorie d'une erreur (KindUnknown si non typée).
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindUnknown
}

// classify convertit une erreur go-ovh / réseau en *Error.
func classify(err error) error {
	if err == nil {
		return nil
	}
	var typed *Error
	if errors.As(err, &typed) {
		return err
	}
	var api *goovh.APIError
	if errors.As(err, &api) {
		return classifyAPI(api)
	}
	var apiVal goovh.APIError
	if errors.As(err, &apiVal) {
		return classifyAPI(&apiVal)
	}
	if errors.Is(err, context.Canceled) {
		return &Error{Kind: KindNetwork, Message: "Opération annulée.", Err: err}
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) {
		return &Error{Kind: KindNetwork, Message: "Impossible de joindre l'API OVH : vérifiez votre connexion.", Err: err}
	}
	return &Error{Kind: KindUnknown, Message: "Erreur inattendue : " + err.Error(), Err: err}
}

func classifyAPI(api *goovh.APIError) *Error {
	msg := strings.ToLower(api.Message)
	e := &Error{Status: api.Code, Err: api}
	switch {
	case api.Code == http.StatusTooManyRequests:
		e.Kind, e.Message = KindRateLimit, "Trop de requêtes : OVH limite le débit. Réessayez dans quelques instants."
	case api.Code == http.StatusUnauthorized || api.Code == http.StatusForbidden:
		e.Kind = KindCredentials
		e.Message = "Identifiants refusés par OVH : clés invalides, expirées, ou droits insuffisants sur ce service."
		if strings.Contains(msg, "expired") {
			e.Message = "Le consumer key a expiré : générez-en un nouveau."
		}
		e.Message += " Détail OVH (" + http.StatusText(api.Code) + ") : " + api.Message
	case api.Code == http.StatusNotFound:
		e.Kind, e.Message = KindNotFound, "Service SMS introuvable : vérifiez le serviceName."
	case strings.Contains(msg, "credit"):
		e.Kind, e.Message = KindInsufficientCredits, "Crédits SMS insuffisants pour cet envoi."
	case strings.Contains(msg, "receiver") || strings.Contains(msg, "destinataire") || strings.Contains(msg, "phone"):
		e.Kind, e.Message = KindInvalidReceiver, "Numéro de destinataire invalide."
	case api.Code == http.StatusBadRequest:
		e.Kind, e.Message = KindInvalidRequest, "Requête refusée par OVH : "+api.Message
	default:
		e.Kind, e.Message = KindUnknown, "Erreur OVH ("+http.StatusText(api.Code)+") : "+api.Message
	}
	return e
}
