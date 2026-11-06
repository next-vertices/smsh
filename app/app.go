// Package app expose le cœur métier au front via les bindings Wails.
// Les méthodes sont volontairement fines : aucune logique métier ici.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"smsh/internal/creds"
	"smsh/internal/i18n"
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
	// Langue des messages du cœur : celle du système (l'interface la réaligne au démarrage).
	i18n.SetLang(i18n.Detect())
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
	svc := service.New(creds.Open(dir), st)
	svc.Logf = fileLogger(filepath.Join(dir, "smsh.log"))
	return &App{svc: svc}, nil
}

// fileLogger écrit des lignes horodatées (diagnostic, sans secrets), fichier tronqué au-delà de 256 Ko.
func fileLogger(path string) func(string, ...any) {
	if fi, err := os.Stat(path); err == nil && fi.Size() > 256<<10 {
		_ = os.Remove(path)
	}
	var mu sync.Mutex
	return func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		defer f.Close()
		fmt.Fprintf(f, "%s %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
	}
}

// Startup est appelé par Wails au démarrage.
func (a *App) Startup(ctx context.Context) { a.ctx = ctx }

func (a *App) callBase() context.Context {
	if a.ctx == nil {
		return context.Background()
	}
	return a.ctx
}

func (a *App) callCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(a.callBase(), callTimeout)
}

// ---- Langue ----

// SystemLanguage renvoie la langue du système, ramenée à une langue prise en charge.
func (a *App) SystemLanguage() string { return i18n.Detect() }

// SetLanguage change la langue des messages du cœur et renvoie la langue appliquée.
func (a *App) SetLanguage(tag string) string { return i18n.SetLang(tag) }

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
		Title: i18n.T("app.csv_pick_title"),
		Filters: []runtime.FileFilter{
			{DisplayName: i18n.T("app.csv_pick_filter"), Pattern: "*.csv;*.txt"},
		},
	})
	if err != nil || path == "" {
		return service.ImportInfo{}, wrap(err)
	}
	info, err := a.svc.ImportCSV(path)
	return info, wrap(err)
}

// csvTemplate : modèle prêt pour Excel (BOM UTF-8, séparateur « ; »), dans la langue courante.
func csvTemplate() string { return "\ufeff" + i18n.T("app.csv_template") }

// SaveCSVTemplate propose d'enregistrer un fichier CSV modèle.
// Renvoie le chemin écrit, ou "" si l'utilisateur annule.
func (a *App) SaveCSVTemplate() (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           i18n.T("app.csv_save_title"),
		DefaultFilename: i18n.T("app.csv_template_name"),
		Filters:         []runtime.FileFilter{{DisplayName: i18n.T("app.csv_save_filter"), Pattern: "*.csv"}},
	})
	if err != nil || path == "" {
		return "", wrap(err)
	}
	return path, wrap(os.WriteFile(path, []byte(csvTemplate()), 0o644))
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

// ---- Historique ----

func (a *App) SyncHistory(days int) (service.SyncResult, error) {
	ctx, cancel := context.WithTimeout(a.callBase(), 5*time.Minute)
	defer cancel()
	r, err := a.svc.SyncHistory(ctx, days)
	return r, wrap(err)
}

func (a *App) SearchHistory(q service.HistoryQuery, offset, limit int) (store.HistoryPage, error) {
	ctx, cancel := a.callCtx()
	defer cancel()
	p, err := a.svc.SearchHistory(ctx, q, offset, limit)
	return p, wrap(err)
}

func (a *App) HistoryMatchingIDs(q service.HistoryQuery) ([]int64, error) {
	ctx, cancel := a.callCtx()
	defer cancel()
	ids, err := a.svc.HistoryMatchingIDs(ctx, q)
	return ids, wrap(err)
}

func (a *App) HistoryFacets() (service.HistoryFacets, error) {
	ctx, cancel := a.callCtx()
	defer cancel()
	f, err := a.svc.HistoryFacets(ctx)
	return f, wrap(err)
}

func (a *App) HistoryDetail(id int64) (store.HistoryItem, error) {
	ctx, cancel := a.callCtx()
	defer cancel()
	h, err := a.svc.HistoryDetail(ctx, id)
	return h, wrap(err)
}

// DeleteHistory supprime des SMS chez OVH et du cache local.
func (a *App) DeleteHistory(ids []int64) (service.DeleteResult, error) {
	// Les suppressions en lot peuvent dépasser le délai d'un appel unitaire.
	ctx, cancel := context.WithTimeout(a.callBase(), 5*time.Minute)
	defer cancel()
	r, err := a.svc.DeleteHistory(ctx, ids)
	return r, wrap(err)
}
