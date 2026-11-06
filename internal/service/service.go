// Package service orchestre creds + ovh + règles SMS. C'est le cœur métier
// partagé par le GUI (app/) et le CLI (cmd/smsh) : ces couches restent fines.
package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"smsh/internal/campaign"
	"smsh/internal/creds"
	"smsh/internal/i18n"
	"smsh/internal/ovh"
	"smsh/internal/sms"
	"smsh/internal/store"
)

// MaxOneShotRecipients est le nombre maximal de destinataires d'un envoi unitaire
// (les campagnes ne sont pas concernées).
const MaxOneShotRecipients = 5

// ClientFactory construit un client OVH à partir de credentials.
type ClientFactory func(ovh.Credentials) (ovh.Client, error)

// Service est le point d'entrée métier.
type Service struct {
	Creds     *creds.Manager
	NewClient ClientFactory
	Now       func() time.Time
	Store     *store.Store
	Runner    *campaign.Runner
	Logf      func(format string, args ...any) // diagnostic (jamais de secrets) ; peut être nil

	syncMu  sync.Mutex
	rates   rateCache
	imports importCache
	batches refreshCache
}

// New crée un Service branché sur la vraie API OVH. Les campagnes interrompues
// par une fermeture de l'application sont remises en pause.
func New(m *creds.Manager, st *store.Store) *Service {
	s := Open(m, st)
	_ = st.ResetInterrupted(context.Background())
	return s
}

// Open crée un Service sans toucher à l'état des campagnes : pour un processus
// secondaire (CLI) qui ne doit pas mettre en pause les campagnes d'une autre instance.
func Open(m *creds.Manager, st *store.Store) *Service {
	s := &Service{Creds: m, NewClient: ovh.New, Now: time.Now, Store: st}
	s.Runner = campaign.NewRunner(st, s.sendCampaignMessage)
	return s
}

func toOVH(a creds.Account, s creds.Secrets) ovh.Credentials {
	return ovh.Credentials{Endpoint: a.Endpoint, AppKey: s.AppKey, AppSecret: s.AppSecret, ConsumerKey: s.ConsumerKey}
}

// ServiceInfo est l'état d'un service SMS.
type ServiceInfo struct {
	Account     string       `json:"account"`
	ServiceName string       `json:"serviceName"`
	Status      string       `json:"status"`
	CreditsLeft float64      `json:"creditsLeft"`
	Senders     []ovh.Sender `json:"senders"`
}

func (s *Service) info(ctx context.Context, name string, a creds.Account, sec creds.Secrets) (ServiceInfo, error) {
	cl, err := s.NewClient(toOVH(a, sec))
	if err != nil {
		return ServiceInfo{}, err
	}
	acc, err := cl.Account(ctx, a.ServiceName)
	if err != nil {
		return ServiceInfo{}, err
	}
	// La liste des expéditeurs est facultative : un token restreint peut ne pas
	// avoir le droit GET /sms/{service}/senders, sans empêcher d'envoyer.
	senders, err := cl.Senders(ctx, a.ServiceName)
	if err != nil || senders == nil {
		senders = []ovh.Sender{}
	}
	return ServiceInfo{Account: name, ServiceName: a.ServiceName, Status: acc.Status, CreditsLeft: acc.CreditsLeft, Senders: senders}, nil
}

// TestConnection vérifie une saisie (sans l'enregistrer) en interrogeant le service.
// Les champs secrets vides d'un compte existant reprennent les valeurs stockées.
func (s *Service) TestConnection(ctx context.Context, in creds.Input) (ServiceInfo, error) {
	a, sec, err := s.Creds.Resolve(in)
	if err != nil {
		return ServiceInfo{}, err
	}
	info, err := s.info(ctx, a.Name, a, sec)
	var oe *ovh.Error
	if errors.As(err, &oe) && oe.Kind == ovh.KindCredentials {
		// Diagnostic sans fuite : seules les longueurs sont affichées.
		return info, &ovh.Error{Kind: oe.Kind, Status: oe.Status, Err: oe.Err,
			Message: i18n.T("svc.diag_lengths", i18n.P{"message": oe.Message, "app": len(sec.AppKey), "secret": len(sec.AppSecret), "consumer": len(sec.ConsumerKey)})}
	}
	return info, err
}

// Overview renvoie crédits et expéditeurs du compte actif.
func (s *Service) Overview(ctx context.Context) (ServiceInfo, error) {
	a, sec, err := s.Creds.Active()
	if err != nil {
		return ServiceInfo{}, err
	}
	return s.info(ctx, a.Name, a, sec)
}

// SendInput est la saisie du formulaire « Envoyer ».
type SendInput struct {
	Message    string `json:"message"`
	Recipients string `json:"recipients"` // texte libre, séparateurs : ligne, virgule, point-virgule
	Sender     string `json:"sender"`
	SendAt     string `json:"sendAt"` // vide = immédiat ; RFC3339 ou "2006-01-02T15:04" (heure locale)
	StopClause bool   `json:"stopClause"`
}

// SendOutcome est le résultat d'un envoi.
type SendOutcome struct {
	IDs            []int64      `json:"ids"`
	Sent           []string     `json:"sent"`     // destinataires acceptés par OVH
	Rejected       []string     `json:"rejected"` // destinataires refusés par OVH
	CreditsRemoved float64      `json:"creditsRemoved"`
	Scheduled      bool         `json:"scheduled"`
	Analysis       sms.Analysis `json:"analysis"`
}

func (s *Service) parseSendAt(v string) (*time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04"} {
		if t, err := time.ParseInLocation(layout, v, time.Local); err == nil {
			return &t, nil
		}
	}
	return nil, &ovh.Error{Kind: ovh.KindInvalidRequest, Message: i18n.T("svc.send_at_invalid")}
}

// Send envoie un SMS one-shot via le compte actif.
func (s *Service) Send(ctx context.Context, in SendInput) (SendOutcome, error) {
	if strings.TrimSpace(in.Message) == "" {
		return SendOutcome{}, &ovh.Error{Kind: ovh.KindInvalidRequest, Message: i18n.T("svc.message_empty")}
	}
	rcpt := sms.ParseReceivers(in.Recipients)
	if len(rcpt.Invalid) > 0 {
		return SendOutcome{}, &ovh.Error{Kind: ovh.KindInvalidReceiver,
			Message: i18n.T("svc.invalid_numbers", i18n.P{"numbers": strings.Join(rcpt.Invalid, ", ")})}
	}
	if len(rcpt.Valid) == 0 {
		return SendOutcome{}, &ovh.Error{Kind: ovh.KindInvalidReceiver, Message: i18n.T("svc.no_recipient")}
	}
	if len(rcpt.Valid) > MaxOneShotRecipients {
		return SendOutcome{}, &ovh.Error{Kind: ovh.KindInvalidRequest,
			Message: i18n.T("svc.too_many_recipients", i18n.P{"max": MaxOneShotRecipients, "got": len(rcpt.Valid)})}
	}
	at, err := s.parseSendAt(in.SendAt)
	if err != nil {
		return SendOutcome{}, err
	}
	a, sec, err := s.Creds.Active()
	if err != nil {
		return SendOutcome{}, err
	}
	cl, err := s.NewClient(toOVH(a, sec))
	if err != nil {
		return SendOutcome{}, err
	}
	an := sms.Analyze(in.Message)
	res, err := cl.Send(ctx, ovh.SendRequest{
		Service: a.ServiceName, Message: in.Message, Receivers: rcpt.Valid,
		Sender: in.Sender, SendAt: at, StopClause: in.StopClause, Unicode: an.Encoding == sms.UCS2,
	})
	if err != nil {
		return SendOutcome{}, err
	}
	return SendOutcome{
		IDs: nonNil(res.IDs), Sent: nonNil(res.ValidReceivers), Rejected: nonNil(res.InvalidReceivers),
		CreditsRemoved: res.CreditsRemoved, Scheduled: at != nil, Analysis: an,
	}, nil
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// Problem est une erreur sérialisable pour l'UI.
type Problem struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// Describe classe n'importe quelle erreur du cœur en Problem affichable.
func Describe(err error) Problem {
	var ve *creds.ValidationError
	var oe *ovh.Error
	switch {
	case errors.Is(err, creds.ErrLocked):
		return Problem{"locked", i18n.T("svc.locked")}
	case errors.Is(err, creds.ErrBadPassphrase):
		return Problem{"bad_passphrase", i18n.T("svc.bad_passphrase")}
	case errors.Is(err, creds.ErrWeakPassphase):
		return Problem{"validation", i18n.T("creds.weak_passphrase")}
	case errors.Is(err, store.ErrNotFound):
		return Problem{"not_found", i18n.T("svc.campaign_not_found")}
	case errors.Is(err, creds.ErrNoAccount):
		return Problem{"no_account", i18n.T("svc.no_account")}
	case errors.As(err, &ve):
		return Problem{"validation", ve.Msg}
	case errors.As(err, &oe):
		return Problem{string(oe.Kind), oe.Message}
	default:
		return Problem{"internal", i18n.T("svc.internal", i18n.P{"error": err.Error()})}
	}
}

func (s *Service) logf(format string, args ...any) {
	if s.Logf != nil {
		s.Logf(format, args...)
	}
}
