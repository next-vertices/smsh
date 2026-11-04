// Package app expose le cœur métier au front via les bindings Wails.
// Les méthodes sont volontairement fines : aucune logique métier ici.
package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"smsh/internal/creds"
	"smsh/internal/ovh"
	"smsh/internal/service"
	"smsh/internal/sms"
	"smsh/internal/store"
)

const callTimeout = 30 * time.Second

// apiError est l'erreur renvoyée au front : son message est un JSON
// {"kind": "...", "message": "..."} que le front décode.
type apiError struct{ service.Problem }

func (e apiError) Error() string {
	b, _ := json.Marshal(e.Problem)
	return string(b)
}

func wrap(err error) error {
	if err == nil {
		return nil
	}
	return apiError{service.Describe(err)}
}

// App est l'objet lié à Wails.
type App struct {
	ctx context.Context
	svc *service.Service
}

// New crée l'App. SMSH_MOCK=1 branche un faux client OVH et un coffre
// en mémoire (démo / développement du front sans credentials réels).
func New() (*App, error) {
	if os.Getenv("SMSH_MOCK") == "1" {
		dir, err := os.MkdirTemp("", "smsh-mock-")
		if err != nil {
			return nil, err
		}
		st, err := store.Open(filepath.Join(dir, "smsh.db"))
		if err != nil {
			return nil, err
		}
		mock := ovh.NewMock()
		svc := service.New(creds.NewManager(dir, creds.NewMemoryVault()), st)
		svc.NewClient = func(ovh.Credentials) (ovh.Client, error) { return mock, nil }
		return &App{svc: svc}, nil
	}
	dir, err := creds.ConfigDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	st, err := store.Open(filepath.Join(dir, "smsh.db"))
	if err != nil {
		return nil, err
	}
	return &App{svc: service.New(creds.Open(dir), st)}, nil
}

// Startup est appelé par Wails au démarrage.
func (a *App) Startup(ctx context.Context) { a.ctx = ctx }

func (a *App) callCtx() (context.Context, context.CancelFunc) {
	base := a.ctx
	if base == nil {
		base = context.Background()
	}
	return context.WithTimeout(base, callTimeout)
}

// ---- Stockage & comptes ----

func (a *App) Status() (creds.Status, error) {
	st, err := a.svc.Creds.Status()
	return st, wrap(err)
}

func (a *App) Unlock(passphrase string) error { return wrap(a.svc.Creds.Unlock(passphrase)) }

func (a *App) ListAccounts() ([]creds.View, error) {
	v, err := a.svc.Creds.List()
	return v, wrap(err)
}

func (a *App) SaveAccount(in creds.Input) (creds.Account, error) {
	acc, err := a.svc.Creds.Save(in)
	return acc, wrap(err)
}

func (a *App) DeleteAccount(id string) error { return wrap(a.svc.Creds.Delete(id)) }

func (a *App) SetActiveAccount(id string) error { return wrap(a.svc.Creds.SetActive(id)) }

func (a *App) Endpoints() []string { return ovh.Endpoints }

// ---- OVH ----

func (a *App) TestConnection(in creds.Input) (service.ServiceInfo, error) {
	ctx, cancel := a.callCtx()
	defer cancel()
	info, err := a.svc.TestConnection(ctx, in)
	return info, wrap(err)
}

func (a *App) Overview() (service.ServiceInfo, error) {
	ctx, cancel := a.callCtx()
	defer cancel()
	info, err := a.svc.Overview(ctx)
	return info, wrap(err)
}

// EstimateCost estime le coût d'un envoi via les endpoints OVH (estimate + tarifs par pays).
func (a *App) EstimateCost(in service.CostInput) (service.CostEstimate, error) {
	ctx, cancel := a.callCtx()
	defer cancel()
	est, err := a.svc.EstimateCost(ctx, in)
	return est, wrap(err)
}

func (a *App) Send(in service.SendInput) (service.SendOutcome, error) {
	ctx, cancel := a.callCtx()
	defer cancel()
	out, err := a.svc.Send(ctx, in)
	return out, wrap(err)
}

// ---- Règles SMS (pures) ----

// MaxRecipients est la limite de destinataires d'un envoi unitaire.
func (a *App) MaxRecipients() int { return service.MaxOneShotRecipients }

func (a *App) AnalyzeMessage(msg string) sms.Analysis { return sms.Analyze(msg) }

func (a *App) ParseRecipients(text string) sms.Receivers { return sms.ParseReceivers(text) }

// ---- Campagnes ----

// PickCSV ouvre le sélecteur de fichier natif puis charge le CSV choisi.
// Renvoie un ImportInfo à ID vide si l'utilisateur annule.
func (a *App) PickCSV() (service.ImportInfo, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Choisir un fichier CSV de destinataires",
		Filters: []runtime.FileFilter{
			{DisplayName: "Fichiers CSV (*.csv;*.txt)", Pattern: "*.csv;*.txt"},
		},
	})
	if err != nil || path == "" {
		return service.ImportInfo{}, wrap(err)
	}
	info, err := a.svc.ImportCSV(path)
	return info, wrap(err)
}

func (a *App) SimulateCampaign(in service.PlanInput) (service.PlanReport, error) {
	ctx, cancel := a.callCtx()
	defer cancel()
	rep, err := a.svc.SimulateCampaign(ctx, in)
	return rep, wrap(err)
}

func (a *App) LaunchCampaign(in service.LaunchInput) (store.Campaign, error) {
	ctx, cancel := a.callCtx()
	defer cancel()
	c, err := a.svc.LaunchCampaign(ctx, in)
	return c, wrap(err)
}

func (a *App) ListCampaigns() ([]store.Campaign, error) {
	ctx, cancel := a.callCtx()
	defer cancel()
	cs, err := a.svc.ListCampaigns(ctx)
	return cs, wrap(err)
}

func (a *App) GetCampaign(id string) (store.Campaign, error) {
	ctx, cancel := a.callCtx()
	defer cancel()
	c, err := a.svc.GetCampaign(ctx, id)
	return c, wrap(err)
}

func (a *App) CampaignRecipients(id, status string, offset, limit int) (service.RecipientsPage, error) {
	ctx, cancel := a.callCtx()
	defer cancel()
	p, err := a.svc.CampaignRecipients(ctx, id, status, offset, limit)
	return p, wrap(err)
}

func (a *App) PauseCampaign(id string) error {
	ctx, cancel := a.callCtx()
	defer cancel()
	return wrap(a.svc.PauseCampaign(ctx, id))
}

func (a *App) ResumeCampaign(id string) error {
	ctx, cancel := a.callCtx()
	defer cancel()
	return wrap(a.svc.ResumeCampaign(ctx, id))
}

func (a *App) CancelCampaign(id string) error {
	ctx, cancel := a.callCtx()
	defer cancel()
	return wrap(a.svc.CancelCampaign(ctx, id))
}

func (a *App) RetryFailed(id string) (int, error) {
	ctx, cancel := a.callCtx()
	defer cancel()
	n, err := a.svc.RetryFailed(ctx, id)
	return n, wrap(err)
}

func (a *App) DeleteCampaign(id string) error {
	ctx, cancel := a.callCtx()
	defer cancel()
	return wrap(a.svc.DeleteCampaign(ctx, id))
}
