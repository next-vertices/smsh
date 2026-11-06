package ovh

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	goovh "github.com/ovh/go-ovh/ovh"

	"smsh/internal/i18n"
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
	KindTooManyResults      Kind = "too_many_results" // liste trop longue : restreindre le filtre
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
		return &Error{Kind: KindNetwork, Message: i18n.T("ovh.canceled"), Err: err}
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) {
		return &Error{Kind: KindNetwork, Message: i18n.T("ovh.network"), Err: err}
	}
	return &Error{Kind: KindUnknown, Message: i18n.T("ovh.unexpected", i18n.P{"error": err.Error()}), Err: err}
}

func classifyAPI(api *goovh.APIError) *Error {
	msg := strings.ToLower(api.Message)
	e := &Error{Status: api.Code, Err: api}
	switch {
	case api.Code == http.StatusTooManyRequests:
		e.Kind, e.Message = KindRateLimit, i18n.T("ovh.rate_limit")
	case api.Code == http.StatusUnauthorized || api.Code == http.StatusForbidden:
		e.Kind = KindCredentials
		e.Message = i18n.T("ovh.credentials")
		if strings.Contains(msg, "expired") {
			e.Message = i18n.T("ovh.consumer_expired")
		}
		e.Message += " " + i18n.T("ovh.detail", i18n.P{"status": http.StatusText(api.Code), "message": api.Message})
	case strings.Contains(msg, "narrow your query"):
		e.Kind, e.Message = KindTooManyResults, i18n.T("ovh.too_many_results")
	case api.Code == http.StatusNotFound:
		e.Kind, e.Message = KindNotFound, i18n.T("ovh.service_not_found")
	case strings.Contains(msg, "credit"):
		e.Kind, e.Message = KindInsufficientCredits, i18n.T("ovh.insufficient_credits")
	case strings.Contains(msg, "receiver") || strings.Contains(msg, "destinataire") || strings.Contains(msg, "phone"):
		e.Kind, e.Message = KindInvalidReceiver, i18n.T("ovh.invalid_receiver")
	case api.Code == http.StatusBadRequest:
		e.Kind, e.Message = KindInvalidRequest, i18n.T("ovh.invalid_request", i18n.P{"message": api.Message})
	default:
		e.Kind, e.Message = KindUnknown, i18n.T("ovh.error", i18n.P{"status": http.StatusText(api.Code), "message": api.Message})
	}
	return e
}
