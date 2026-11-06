package campaign

import (
	"context"
	"errors"
	"sync"
	"time"

	"smsh/internal/i18n"
	"smsh/internal/ovh"
	"smsh/internal/store"
)

// SendFunc envoie le message d'un destinataire (fourni par le service : il connaît le compte).
type SendFunc func(ctx context.Context, c store.Campaign, r store.Recipient) (ovh.SendResult, error)

var (
	errPause  = errors.New("pause demandée")
	errCancel = errors.New("annulation demandée")
)

// DefaultRate : débit par défaut (SMS/seconde) quand la campagne n'en précise pas.
const DefaultRate = 3.0

// Runner exécute les campagnes locales : un SMS par destinataire, débit limité,
// avec pause, reprise et annulation. L'état est persisté à chaque envoi : une
// campagne interrompue peut reprendre exactement où elle s'est arrêtée.
type Runner struct {
	Store *store.Store
	Send  SendFunc
	// Sleep attend d. Remplaçable en test. Doit retourner une erreur si ctx est annulé.
	Sleep func(ctx context.Context, d time.Duration) error
	// Backoff : attentes successives avant de retenter après rate limit / erreur réseau.
	// Une fois épuisées, la campagne est mise en pause.
	Backoff []time.Duration

	mu   sync.Mutex
	runs map[string]*run
}

type run struct {
	cancel context.CancelCauseFunc
	done   chan struct{}
}

// NewRunner crée un Runner avec les réglages par défaut.
func NewRunner(s *store.Store, send SendFunc) *Runner {
	return &Runner{
		Store:   s,
		Send:    send,
		Sleep:   sleepCtx,
		Backoff: []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second, 30 * time.Second, 60 * time.Second},
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-t.C:
		return nil
	}
}

// Start lance (ou reprend) une campagne en pause ou en attente.
func (r *Runner) Start(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, running := r.runs[id]; running {
		return nil
	}
	ctx := context.Background()
	c, err := r.Store.GetCampaign(ctx, id)
	if err != nil {
		return err
	}
	if c.Mode != store.ModeLocal {
		return errors.New(i18n.T("camp.is_batch"))
	}
	if c.Status != store.StatusPending && c.Status != store.StatusPaused {
		return errors.New(i18n.T("camp.cannot_start", i18n.P{"status": c.Status}))
	}
	if err := r.Store.SetStatus(ctx, id, store.StatusRunning, ""); err != nil {
		return err
	}
	if r.runs == nil {
		r.runs = map[string]*run{}
	}
	runCtx, cancel := context.WithCancelCause(ctx)
	rn := &run{cancel: cancel, done: make(chan struct{})}
	r.runs[id] = rn
	go r.loop(runCtx, rn, c)
	return nil
}

// Pause suspend l'envoi après le SMS en cours.
func (r *Runner) Pause(id string) error { return r.stop(id, errPause, store.StatusPaused) }

// Cancel arrête définitivement la campagne (les destinataires restants ne seront pas envoyés).
func (r *Runner) Cancel(id string) error { return r.stop(id, errCancel, store.StatusCanceled) }

func (r *Runner) stop(id string, cause error, status string) error {
	r.mu.Lock()
	rn := r.runs[id]
	r.mu.Unlock()
	if rn != nil {
		rn.cancel(cause)
		<-rn.done
		return nil
	}
	c, err := r.Store.GetCampaign(context.Background(), id)
	if err != nil {
		return err
	}
	if !store.Active(c.Status) {
		return errors.New(i18n.T("camp.already_done", i18n.P{"status": c.Status}))
	}
	return r.Store.SetStatus(context.Background(), id, status, "")
}

// Wait bloque jusqu'à la fin de l'exécution en cours de la campagne (tests).
func (r *Runner) Wait(id string) {
	r.mu.Lock()
	rn := r.runs[id]
	r.mu.Unlock()
	if rn != nil {
		<-rn.done
	}
}

// Running indique si la campagne est en cours d'exécution dans ce processus.
func (r *Runner) Running(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.runs[id]
	return ok
}

func (r *Runner) loop(ctx context.Context, rn *run, c store.Campaign) {
	defer func() {
		r.mu.Lock()
		delete(r.runs, c.ID)
		r.mu.Unlock()
		close(rn.done)
	}()
	bg := context.Background()
	status, msg := r.execute(ctx, c)
	if status == "" { // arrêt demandé de l'extérieur
		switch context.Cause(ctx) {
		case errCancel:
			status = store.StatusCanceled
		default:
			status = store.StatusPaused
		}
	}
	_ = r.Store.SetStatus(bg, c.ID, status, msg)
}

// execute envoie les destinataires un par un ; renvoie le statut final
// (vide si l'arrêt vient d'une demande de pause/annulation).
func (r *Runner) execute(ctx context.Context, c store.Campaign) (string, string) {
	rate := c.Rate
	if rate <= 0 {
		rate = DefaultRate
	}
	interval := time.Duration(float64(time.Second) / rate)
	bg := context.Background()
	var last time.Time
	for {
		if ctx.Err() != nil {
			return "", ""
		}
		rs, err := r.Store.NextPending(bg, c.ID, 1)
		if err != nil {
			return store.StatusPaused, i18n.T("camp.db_error", i18n.P{"error": err.Error()})
		}
		if len(rs) == 0 {
			return store.StatusCompleted, ""
		}
		if !last.IsZero() {
			if wait := interval - time.Since(last); wait > 0 {
				if r.Sleep(ctx, wait) != nil {
					return "", ""
				}
			}
		}
		status, msg, next := r.sendOne(ctx, c, rs[0])
		last = time.Now()
		if !next {
			return status, msg
		}
	}
}

// sendOne envoie un destinataire avec reprises. next=false : arrêter la campagne
// avec (status, msg) ; status vide = arrêt demandé.
func (r *Runner) sendOne(ctx context.Context, c store.Campaign, rec store.Recipient) (status, msg string, next bool) {
	bg := context.Background()
	for attempt := 0; ; attempt++ {
		// L'appel en cours n'est pas interrompu par une pause : on ne sait plus
		// sinon si le SMS est parti. La pause prend effet entre deux SMS.
		res, err := r.Send(context.WithoutCancel(ctx), c, rec)
		if err == nil {
			if len(res.ValidReceivers) == 0 && len(res.IDs) == 0 || contains(res.InvalidReceivers, rec.Phone) {
				_ = r.Store.MarkRecipient(bg, c.ID, rec.Idx, store.RecipientFailed, i18n.T("camp.number_refused"), 0, 0)
				return "", "", true
			}
			var id int64
			if len(res.IDs) > 0 {
				id = res.IDs[0]
			}
			_ = r.Store.MarkRecipient(bg, c.ID, rec.Idx, store.RecipientSent, "", id, res.CreditsRemoved)
			return "", "", true
		}
		switch ovh.KindOf(err) {
		case ovh.KindRateLimit, ovh.KindNetwork:
			if attempt >= len(r.Backoff) {
				return store.StatusPaused, err.Error() + " — campagne mise en pause, reprenez-la plus tard.", false
			}
			if r.Sleep(ctx, r.Backoff[attempt]) != nil {
				return "", "", false
			}
		case ovh.KindInsufficientCredits, ovh.KindCredentials, ovh.KindNotFound:
			// Erreur bloquante : inutile d'insister sur les suivants.
			return store.StatusPaused, err.Error() + " — campagne mise en pause.", false
		default:
			_ = r.Store.MarkRecipient(bg, c.ID, rec.Idx, store.RecipientFailed, err.Error(), 0, 0)
			return "", "", true
		}
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
