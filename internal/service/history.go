package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"smsh/internal/creds"
	"smsh/internal/ovh"
	"smsh/internal/sms"
	"smsh/internal/store"
)

const (
	defaultSyncDays   = 30
	maxFetchPerSync   = 300 // détails récupérés par synchronisation (une requête chacun)
	refreshRecentDays = 3   // les SMS récents sans accusé sont ré-interrogés
	refreshMax        = 100
	apiWorkers        = 4
	maxDeleteBatch    = 2000
	pageSizeMax       = 200
)

// historyCtx renvoie le compte actif, son service et un client OVH.
func (s *Service) historyCtx() (creds.Account, ovh.Client, error) {
	a, sec, err := s.Creds.Active()
	if err != nil {
		return a, nil, err
	}
	cl, err := s.NewClient(toOVH(a, sec))
	return a, cl, err
}

func toHistory(accountID string, o ovh.Outgoing) store.HistoryItem {
	return store.HistoryItem{
		AccountID: accountID, ID: o.ID, CreatedAt: o.CreatedAt, SentAt: o.SentAt, DeliveredAt: o.DeliveredAt,
		Receiver: o.Receiver, Sender: o.Sender, Message: o.Message, Credits: o.Credits, Parts: max(o.Parts, 1),
		Tag: o.Tag, BatchID: o.BatchID, Receipt: o.Receipt, PTT: o.PTT, Differed: o.Differed,
	}
}

// SyncResult résume une synchronisation de l'historique avec OVH.
type SyncResult struct {
	Listed    int      `json:"listed"`    // SMS connus d'OVH sur la période
	Fetched   int      `json:"fetched"`   // détails téléchargés
	Remaining int      `json:"remaining"` // nouveaux SMS restant à télécharger : relancer la synchronisation
	Removed   int      `json:"removed"`   // SMS supprimés chez OVH, retirés du cache
	Errors    []string `json:"errors"`
}

// SyncHistory met à jour le cache local : télécharge les nouveaux SMS des `days` derniers jours,
// rafraîchit l'accusé de réception des plus récents, et retire ceux supprimés chez OVH.
// Au plus 300 détails par appel : si Remaining > 0, relancez pour poursuivre.
func (s *Service) SyncHistory(ctx context.Context, days int) (SyncResult, error) {
	res := SyncResult{Errors: []string{}}
	if days <= 0 {
		days = defaultSyncDays
	}
	acc, cl, err := s.historyCtx()
	if err != nil {
		return res, err
	}
	to := s.Now()
	from := to.AddDate(0, 0, -days)
	ids, err := listOutgoingWindowed(ctx, cl, acc.ServiceName, from, to)
	if err != nil {
		return res, err
	}
	res.Listed = len(ids)
	remote := make(map[int64]bool, len(ids))
	for _, id := range ids {
		remote[id] = true
	}
	cached, err := s.Store.HistoryIDsSince(ctx, acc.ID, from)
	if err != nil {
		return res, err
	}

	// Supprimés chez OVH : on les retire du cache local.
	var gone []int64
	for id := range cached {
		if !remote[id] {
			gone = append(gone, id)
		}
	}
	if len(gone) > 0 {
		if err := s.Store.DeleteHistory(ctx, acc.ID, gone); err != nil {
			return res, err
		}
		res.Removed = len(gone)
	}

	// À télécharger : les nouveaux, puis les récents sans accusé de réception.
	var todo []int64
	isNew := map[int64]bool{}
	for _, id := range ids {
		if !cached[id] {
			todo = append(todo, id)
			isNew[id] = true
		}
	}
	recent, err := s.Store.HistoryUndelivered(ctx, acc.ID, s.Now().AddDate(0, 0, -refreshRecentDays), refreshMax)
	if err != nil {
		return res, err
	}
	for _, id := range recent {
		if remote[id] {
			todo = append(todo, id)
		}
	}
	if len(todo) > maxFetchPerSync {
		todo = todo[:maxFetchPerSync]
	}

	items, errs := s.fetchOutgoing(ctx, cl, acc, todo)
	if err := s.Store.UpsertHistory(ctx, items); err != nil {
		return res, err
	}
	res.Fetched = len(items)
	res.Errors = append(res.Errors, errs...)
	storedNew := 0
	for _, it := range items {
		if isNew[it.ID] {
			storedNew++
		}
	}
	res.Remaining = len(isNew) - storedNew
	return res, nil
}

// minWindow : en dessous, on n'essaie plus de découper la période.
const minWindow = time.Minute

// listOutgoingWindowed liste les SMS d'une période. OVH refuse les listes trop longues
// (« please narrow your query filter ») : la période est alors coupée en deux, récursivement.
func listOutgoingWindowed(ctx context.Context, cl ovh.Client, service string, from, to time.Time) ([]int64, error) {
	seen := map[int64]bool{}
	var out []int64
	var walk func(a, b time.Time) error
	walk = func(a, b time.Time) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		ids, err := cl.ListOutgoing(ctx, service, ovh.OutgoingFilter{From: &a, To: &b})
		if err == nil {
			for _, id := range ids {
				if !seen[id] {
					seen[id] = true
					out = append(out, id)
				}
			}
			return nil
		}
		if ovh.KindOf(err) != ovh.KindTooManyResults || b.Sub(a) <= minWindow {
			return err
		}
		mid := a.Add(b.Sub(a) / 2)
		if err := walk(a, mid); err != nil {
			return err
		}
		return walk(mid, b)
	}
	return out, walk(from, to)
}

// fetchOutgoing télécharge les détails avec quelques requêtes en parallèle.
// Une limite de débit (429) interrompt le téléchargement.
func (s *Service) fetchOutgoing(ctx context.Context, cl ovh.Client, acc creds.Account, ids []int64) ([]store.HistoryItem, []string) {
	var (
		mu      sync.Mutex
		items   []store.HistoryItem
		errs    []string
		stopped bool
		wg      sync.WaitGroup
	)
	jobs := make(chan int64)
	for w := 0; w < apiWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				mu.Lock()
				skip := stopped
				mu.Unlock()
				if skip {
					continue
				}
				o, err := cl.GetOutgoing(ctx, acc.ServiceName, id)
				mu.Lock()
				switch {
				case err == nil:
					items = append(items, toHistory(acc.ID, o))
				case ovh.KindOf(err) == ovh.KindNotFound:
					// supprimé entre-temps : ignoré
				default:
					errs = append(errs, err.Error())
					if k := ovh.KindOf(err); k == ovh.KindRateLimit || k == ovh.KindCredentials || k == ovh.KindNetwork {
						stopped = true
					}
				}
				mu.Unlock()
			}
		}()
	}
	for _, id := range ids {
		jobs <- id
	}
	close(jobs)
	wg.Wait()
	if len(errs) > 3 {
		errs = append(errs[:3], "…")
	}
	return items, errs
}

// ---------- recherche ----------

// HistoryQuery est la recherche saisie dans l'écran Historique.
type HistoryQuery struct {
	Query    string `json:"query"`
	Receiver string `json:"receiver"`
	Status   string `json:"status"` // delivered | sent | scheduled
	Sender   string `json:"sender"`
	Tag      string `json:"tag"`
	DateFrom string `json:"dateFrom"` // YYYY-MM-DD (heure locale), inclus
	DateTo   string `json:"dateTo"`   // YYYY-MM-DD (heure locale), inclus
}

func (s *Service) filterFor(accountID string, q HistoryQuery) (store.HistoryFilter, error) {
	f := store.HistoryFilter{AccountID: accountID, Query: q.Query, Status: q.Status, Sender: q.Sender, Tag: q.Tag}
	if r := strings.TrimSpace(q.Receiver); r != "" {
		if n, ok := sms.NormalizeNumber(r); ok {
			r = n
		}
		f.Receiver = r
	}
	parse := func(v string) (*time.Time, error) {
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		t, err := time.ParseInLocation("2006-01-02", v, time.Local)
		if err != nil {
			return nil, invalid("Date invalide : %q (attendu AAAA-MM-JJ).", v)
		}
		return &t, nil
	}
	var err error
	if f.From, err = parse(q.DateFrom); err != nil {
		return f, err
	}
	if f.To, err = parse(q.DateTo); err != nil {
		return f, err
	}
	if f.To != nil {
		end := f.To.AddDate(0, 0, 1).Add(-time.Nanosecond)
		f.To = &end
	}
	if f.From != nil && f.To != nil && f.To.Before(*f.From) {
		return f, invalid("La date de fin précède la date de début.")
	}
	return f, nil
}

func (s *Service) activeAccountID() (string, error) {
	a, _, err := s.Creds.Active()
	return a.ID, err
}

// SearchHistory interroge le cache local (instantané).
func (s *Service) SearchHistory(ctx context.Context, q HistoryQuery, offset, limit int) (store.HistoryPage, error) {
	id, err := s.activeAccountID()
	if err != nil {
		return store.HistoryPage{}, err
	}
	f, err := s.filterFor(id, q)
	if err != nil {
		return store.HistoryPage{}, err
	}
	if limit <= 0 || limit > pageSizeMax {
		limit = 50
	}
	return s.Store.QueryHistory(ctx, f, max(offset, 0), limit)
}

// HistoryMatchingIDs renvoie les identifiants de tous les résultats d'une recherche
// (pour « tout sélectionner »), plafonnés à 2000.
func (s *Service) HistoryMatchingIDs(ctx context.Context, q HistoryQuery) ([]int64, error) {
	id, err := s.activeAccountID()
	if err != nil {
		return nil, err
	}
	f, err := s.filterFor(id, q)
	if err != nil {
		return nil, err
	}
	return s.Store.HistoryIDs(ctx, f, maxDeleteBatch)
}

// HistoryFacets liste les expéditeurs et tags connus (pour les filtres).
type HistoryFacets struct {
	Senders []string `json:"senders"`
	Tags    []string `json:"tags"`
}

func (s *Service) HistoryFacets(ctx context.Context) (HistoryFacets, error) {
	id, err := s.activeAccountID()
	if err != nil {
		return HistoryFacets{}, err
	}
	senders, tags, err := s.Store.HistoryFacets(ctx, id)
	return HistoryFacets{Senders: senders, Tags: tags}, err
}

// HistoryDetail renvoie un SMS, après avoir rafraîchi son état (accusé de réception) auprès d'OVH
// si possible ; en cas d'échec réseau, la version en cache est renvoyée.
func (s *Service) HistoryDetail(ctx context.Context, id int64) (store.HistoryItem, error) {
	acc, cl, err := s.historyCtx()
	if err != nil {
		return store.HistoryItem{}, err
	}
	cached, cacheErr := s.Store.GetHistory(ctx, acc.ID, id)
	o, err := cl.GetOutgoing(ctx, acc.ServiceName, id)
	if err != nil {
		if cacheErr == nil {
			return cached, nil
		}
		return store.HistoryItem{}, err
	}
	h := toHistory(acc.ID, o)
	if err := s.Store.UpsertHistory(ctx, []store.HistoryItem{h}); err != nil {
		return h, err
	}
	return s.Store.GetHistory(ctx, acc.ID, id)
}

// ---------- suppression ----------

// DeleteFailure décrit un SMS non supprimé.
type DeleteFailure struct {
	ID    int64  `json:"id"`
	Error string `json:"error"`
}

// DeleteResult est le bilan d'une suppression (unitaire ou en lot).
type DeleteResult struct {
	Deleted int             `json:"deleted"`
	Failed  []DeleteFailure `json:"failed"`
}

// DeleteHistory supprime des SMS chez OVH puis du cache local. Un SMS déjà
// absent chez OVH (404) compte comme supprimé. Une limite de débit interrompt le lot.
func (s *Service) DeleteHistory(ctx context.Context, ids []int64) (DeleteResult, error) {
	res := DeleteResult{Failed: []DeleteFailure{}}
	if len(ids) == 0 {
		return res, nil
	}
	if len(ids) > maxDeleteBatch {
		return res, invalid("Suppression limitée à %d SMS à la fois.", maxDeleteBatch)
	}
	acc, cl, err := s.historyCtx()
	if err != nil {
		return res, err
	}
	var (
		mu      sync.Mutex
		done    []int64
		stopped bool
		wg      sync.WaitGroup
	)
	jobs := make(chan int64)
	for w := 0; w < apiWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				mu.Lock()
				skip := stopped
				mu.Unlock()
				if skip {
					mu.Lock()
					res.Failed = append(res.Failed, DeleteFailure{ID: id, Error: "Interrompu (limite de débit ou erreur bloquante)."})
					mu.Unlock()
					continue
				}
				err := cl.DeleteOutgoing(ctx, acc.ServiceName, id)
				mu.Lock()
				switch {
				case err == nil || ovh.KindOf(err) == ovh.KindNotFound:
					done = append(done, id)
				default:
					res.Failed = append(res.Failed, DeleteFailure{ID: id, Error: err.Error()})
					if k := ovh.KindOf(err); k == ovh.KindRateLimit || k == ovh.KindCredentials {
						stopped = true
					}
				}
				mu.Unlock()
			}
		}()
	}
	for _, id := range ids {
		jobs <- id
	}
	close(jobs)
	wg.Wait()
	res.Deleted = len(done)
	if err := s.Store.DeleteHistory(ctx, acc.ID, done); err != nil {
		return res, errors.Join(errors.New("SMS supprimés chez OVH mais cache local non mis à jour"), err)
	}
	return res, nil
}
