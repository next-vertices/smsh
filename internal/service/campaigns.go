package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
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

const (
	maxImportBytes = 20 << 20 // 20 Mo
	maxImports     = 5        // imports gardés en mémoire
	batchRefresh   = 3 * time.Second
	sampleCount    = 5
	issueCount     = 10
)

func invalid(format string, a ...any) error {
	return &creds.ValidationError{Msg: fmt.Sprintf(format, a...)}
}

// ---------- import CSV ----------

// ImportInfo décrit un CSV chargé (les données restent côté Go, référencées par ID).
type ImportInfo struct {
	ID          string     `json:"id"`
	File        string     `json:"file"`
	Headers     []string   `json:"headers"`
	Rows        int        `json:"rows"`
	Sample      [][]string `json:"sample"`
	PhoneColumn int        `json:"phoneColumn"` // -1 si non détectée
}

type importCache struct {
	mu    sync.Mutex
	order []string
	m     map[string]campaign.Table
}

func (c *importCache) put(id string, t campaign.Table) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]campaign.Table{}
	}
	c.m[id] = t
	c.order = append(c.order, id)
	for len(c.order) > maxImports {
		delete(c.m, c.order[0])
		c.order = c.order[1:]
	}
}

func (c *importCache) get(id string) (campaign.Table, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.m[id]
	return t, ok
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ImportCSV charge un fichier CSV (la première ligne contient les en-têtes).
func (s *Service) ImportCSV(path string) (ImportInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return ImportInfo{}, invalid("%s", i18n.T("svc.file_unreadable", i18n.P{"error": err.Error()}))
	}
	if fi.Size() > maxImportBytes {
		return ImportInfo{}, invalid("%s", i18n.T("svc.file_too_big", i18n.P{"mb": maxImportBytes >> 20}))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ImportInfo{}, invalid("%s", i18n.T("svc.file_unreadable", i18n.P{"error": err.Error()}))
	}
	t, err := campaign.ParseCSV(data)
	if err != nil {
		return ImportInfo{}, invalid("%v", err)
	}
	info := ImportInfo{
		ID: newID(), File: filepath.Base(path), Headers: t.Headers, Rows: len(t.Rows),
		Sample: t.Rows[:min(len(t.Rows), sampleCount)], PhoneColumn: campaign.DetectPhoneColumn(t),
	}
	s.imports.put(info.ID, t)
	return info, nil
}

// ---------- simulation (dry-run) ----------

// PlanInput décrit une campagne à simuler ou lancer.
type PlanInput struct {
	ImportID    string  `json:"importId"`
	PhoneColumn int     `json:"phoneColumn"`
	Template    string  `json:"template"`
	Mode        string  `json:"mode"` // "batch" | "local" ; vide = automatique
	StopClause  bool    `json:"stopClause"`
	SenderType  string  `json:"senderType"`
	Rate        float64 `json:"rate"` // SMS/s, mode local
}

// PlanSample est l'aperçu d'un message rendu.
type PlanSample struct {
	Phone    string `json:"phone"`
	Message  string `json:"message"`
	Parts    int    `json:"parts"`
	Encoding string `json:"encoding"`
}

// PlanReport est le résultat d'une simulation : rien n'est envoyé.
type PlanReport struct {
	Mode            string              `json:"mode"` // mode effectivement retenu
	Recipients      int                 `json:"recipients"`
	InvalidCount    int                 `json:"invalidCount"`
	Invalid         []campaign.RowIssue `json:"invalid"`
	Dupes           int                 `json:"dupes"`
	Variables       []string            `json:"variables"`
	Samples         []PlanSample        `json:"samples"`
	TotalParts      int                 `json:"totalParts"`
	MaxParts        int                 `json:"maxParts"`
	Lines           []CostLine          `json:"lines"`
	TotalCredits    float64             `json:"totalCredits"`
	PriceMin        float64             `json:"priceMin"`
	PriceMax        float64             `json:"priceMax"`
	Currency        string              `json:"currency"`
	UnknownCountry  int                 `json:"unknownCountry"` // numéros non chiffrés (pays inconnu)
	DurationSeconds float64             `json:"durationSeconds"`
	Blocking        []string            `json:"blocking"` // empêchent le lancement
	Warnings        []string            `json:"warnings"`
}

type plan struct {
	report   PlanReport
	contacts []campaign.Contact
	messages []string // un par contact (mode local) ; vide en mode batch
}

func (s *Service) makePlan(ctx context.Context, in PlanInput) (plan, error) {
	p := plan{report: PlanReport{Invalid: []campaign.RowIssue{}, Variables: []string{}, Samples: []PlanSample{},
		Lines: []CostLine{}, Blocking: []string{}, Warnings: []string{}}}
	r := &p.report

	s.logf("simulate: début (mode=%q)", in.Mode)
	t, ok := s.imports.get(in.ImportID)
	if !ok {
		return p, invalid("%s", i18n.T("svc.csv_not_loaded"))
	}
	contacts, err := campaign.ExtractContacts(t, in.PhoneColumn)
	if err != nil {
		return p, invalid("%v", err)
	}
	p.contacts = contacts.Valid
	r.Recipients, r.Dupes, r.InvalidCount = len(contacts.Valid), contacts.Dupes, len(contacts.Invalid)
	r.Invalid = append(r.Invalid, contacts.Invalid[:min(len(contacts.Invalid), issueCount)]...) // jamais nil : le front itère dessus
	if r.InvalidCount > 0 {
		r.Warnings = append(r.Warnings, i18n.T("svc.rows_ignored", i18n.P{"count": r.InvalidCount}))
	}
	if r.Recipients == 0 {
		r.Blocking = append(r.Blocking, i18n.T("svc.no_valid_recipient"))
	}

	if strings.TrimSpace(in.Template) == "" {
		r.Blocking = append(r.Blocking, i18n.T("svc.message_empty"))
		return p, nil
	}
	vars := campaign.Variables(in.Template)
	r.Variables = append(r.Variables, vars...)
	if unknown := campaign.Unknown(in.Template, t.Headers); len(unknown) > 0 {
		r.Blocking = append(r.Blocking, "Variable(s) absente(s) du fichier : {"+strings.Join(unknown, "}, {")+"}")
	}

	// Mode : les variables imposent le moteur local (un batch envoie le même message à tous).
	r.Mode = in.Mode
	switch {
	case len(vars) > 0 && r.Mode == store.ModeBatch:
		r.Blocking = append(r.Blocking, i18n.T("svc.batch_no_vars"))
	case r.Mode == "":
		r.Mode = store.ModeBatch
		if len(vars) > 0 {
			r.Mode = store.ModeLocal
		}
	}

	// Rendu des messages et segments (local : segments propres à chaque destinataire).
	agg := map[string]*countryAgg{}
	var renderErr error
	for i, c := range contacts.Valid {
		msg := in.Template
		if len(vars) > 0 {
			if msg, err = campaign.Render(in.Template, c.Vars); err != nil {
				renderErr = err
				break
			}
		}
		if strings.TrimSpace(msg) == "" {
			r.Warnings = append(r.Warnings, i18n.T("svc.empty_message_for", i18n.P{"phone": c.Phone}))
		}
		an := sms.Analyze(msg)
		if r.Mode == store.ModeLocal {
			p.messages = append(p.messages, msg)
		}
		r.TotalParts += an.Segments
		r.MaxParts = max(r.MaxParts, an.Segments)
		if i < sampleCount {
			enc := "7bits"
			if an.Encoding == sms.UCS2 {
				enc = "unicode"
			}
			r.Samples = append(r.Samples, PlanSample{Phone: c.Phone, Message: msg, Parts: an.Segments, Encoding: enc})
		}
		if cc := sms.CountryOf(c.Phone); cc == "" {
			r.UnknownCountry++
		} else {
			if agg[cc] == nil {
				agg[cc] = &countryAgg{}
			}
			agg[cc].recipients++
			agg[cc].parts += an.Segments
		}
	}
	if renderErr != nil {
		r.Blocking = append(r.Blocking, renderErr.Error())
		return p, nil
	}
	if r.MaxParts > 1 {
		r.Warnings = append(r.Warnings, i18n.T("svc.multi_segment", i18n.P{"max": r.MaxParts}))
	}
	if in.StopClause {
		r.Warnings = append(r.Warnings, i18n.T("svc.stop_not_counted"))
	}
	if r.UnknownCountry > 0 {
		r.Warnings = append(r.Warnings, i18n.T("svc.unknown_country", i18n.P{"count": r.UnknownCountry}))
	}

	s.logf("simulate: messages rendus (%d destinataires)", r.Recipients)
	// Tarifs : nécessitent un compte actif ; une indisponibilité n'empêche pas la simulation.
	if a, sec, err := s.Creds.Active(); err != nil {
		r.Warnings = append(r.Warnings, i18n.T("svc.cost_no_account"))
	} else if cl, err := s.NewClient(toOVH(a, sec)); err == nil {
		s.logf("simulate: client créé, tarifs…")
		pr, err := s.priceLines(ctx, cl, agg)
		s.logf("simulate: tarifs terminés (err=%v)", err)
		if err != nil {
			r.Warnings = append(r.Warnings, i18n.T("svc.cost_error", i18n.P{"error": err.Error()}))
		} else {
			r.Lines, r.TotalCredits, r.PriceMin, r.PriceMax, r.Currency = pr.Lines, pr.TotalCredits, pr.PriceMin, pr.PriceMax, pr.Currency
			s.logf("simulate: solde…")
			ai, aerr := cl.Account(ctx, a.ServiceName)
			s.logf("simulate: solde terminé (err=%v)", aerr)
			if a := ai; aerr == nil && r.TotalCredits > a.CreditsLeft {
				r.Blocking = append(r.Blocking, i18n.T("svc.insufficient", i18n.P{"needed": r.TotalCredits, "available": a.CreditsLeft}))
			}
		}
	}
	s.logf("simulate: terminé")
	if r.Mode == store.ModeLocal {
		rate := in.Rate
		if rate <= 0 {
			rate = campaign.DefaultRate
		}
		r.DurationSeconds = float64(r.Recipients) / rate
	}
	return p, nil
}

// SimulateCampaign réalise un dry-run complet : import, rendu des messages, segments,
// coût par pays, durée estimée et problèmes bloquants. Rien n'est envoyé ni enregistré.
func (s *Service) SimulateCampaign(ctx context.Context, in PlanInput) (PlanReport, error) {
	p, err := s.makePlan(ctx, in)
	return p.report, err
}

// ---------- lancement ----------

// LaunchInput est la demande de création d'une campagne.
type LaunchInput struct {
	PlanInput
	Name   string `json:"name"`
	Sender string `json:"sender"`
	SendAt string `json:"sendAt"` // vide = immédiat
}

// LaunchCampaign crée la campagne et la démarre.
//   - batch : un batch OVH est créé (OVH gère l'envoi, même application fermée) ;
//   - local : un SMS par destinataire, via le moteur local (débit limité, pause/reprise).
func (s *Service) LaunchCampaign(ctx context.Context, in LaunchInput) (store.Campaign, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return store.Campaign{}, invalid("%s", i18n.T("svc.name_required"))
	}
	p, err := s.makePlan(ctx, in.PlanInput)
	if err != nil {
		return store.Campaign{}, err
	}
	if len(p.report.Blocking) > 0 {
		return store.Campaign{}, invalid("%s", strings.Join(p.report.Blocking, " "))
	}
	at, err := s.parseSendAt(in.SendAt)
	if err != nil {
		return store.Campaign{}, err
	}
	acc, sec, err := s.Creds.Active()
	if err != nil {
		return store.Campaign{}, err
	}
	c := store.Campaign{
		ID: newID(), Name: name, Mode: p.report.Mode, AccountID: acc.ID, Template: in.Template,
		Sender: in.Sender, StopClause: in.StopClause, SendAt: at, Rate: in.Rate,
		Status: store.StatusPending, Total: len(p.contacts),
	}
	if c.Rate <= 0 {
		c.Rate = campaign.DefaultRate
	}

	if c.Mode == store.ModeBatch {
		cl, err := s.NewClient(toOVH(acc, sec))
		if err != nil {
			return store.Campaign{}, err
		}
		to := make([]string, len(p.contacts))
		for i, ct := range p.contacts {
			to[i] = ct.Phone
		}
		b, err := cl.CreateBatch(ctx, ovh.BatchRequest{
			Service: acc.ServiceName, Name: name, Message: in.Template, Sender: in.Sender,
			To: to, SendAt: at, StopClause: in.StopClause, Tag: c.ID,
		})
		if err != nil {
			return store.Campaign{}, err
		}
		c.BatchID, c.Status, c.Credits = b.ID, store.StatusRunning, b.EstimatedCredits
		if err := s.Store.CreateCampaign(ctx, c, nil); err != nil {
			return store.Campaign{}, fmt.Errorf("%s: %w", i18n.T("svc.batch_not_saved", i18n.P{"id": b.ID}), err)
		}
		return c, nil
	}

	rs := make([]store.Recipient, len(p.contacts))
	for i, ct := range p.contacts {
		rs[i] = store.Recipient{Idx: i, Phone: ct.Phone, Vars: ct.Vars, Message: p.messages[i]}
	}
	if err := s.Store.CreateCampaign(ctx, c, rs); err != nil {
		return store.Campaign{}, err
	}
	if err := s.Runner.Start(c.ID); err != nil {
		return store.Campaign{}, err
	}
	return s.Store.GetCampaign(ctx, c.ID)
}

// sendCampaignMessage est la SendFunc du moteur local.
func (s *Service) sendCampaignMessage(ctx context.Context, c store.Campaign, r store.Recipient) (ovh.SendResult, error) {
	acc, sec, err := s.Creds.Load(c.AccountID)
	if err != nil {
		return ovh.SendResult{}, &ovh.Error{Kind: ovh.KindCredentials, Message: i18n.T("svc.campaign_account_unavailable", i18n.P{"error": err.Error()}), Err: err}
	}
	cl, err := s.NewClient(toOVH(acc, sec))
	if err != nil {
		return ovh.SendResult{}, err
	}
	var at *time.Time
	if c.SendAt != nil && c.SendAt.After(s.Now()) {
		at = c.SendAt
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return cl.Send(ctx, ovh.SendRequest{
		Service: acc.ServiceName, Message: r.Message, Receivers: []string{r.Phone}, Sender: c.Sender,
		SendAt: at, StopClause: c.StopClause, Unicode: sms.Analyze(r.Message).Encoding == sms.UCS2, Tag: c.ID,
	})
}

// ---------- suivi et contrôle ----------

type refreshCache struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func (r *refreshCache) due(id string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.last == nil {
		r.last = map[string]time.Time{}
	}
	if now.Sub(r.last[id]) < batchRefresh {
		return false
	}
	r.last[id] = now
	return true
}

func batchStatus(ovhStatus string) string {
	switch ovhStatus {
	case "COMPLETED":
		return store.StatusCompleted
	case "FAILED":
		return store.StatusFailed
	case "TO_CANCEL", "CANCELING", "CANCELED":
		return store.StatusCanceled
	default: // PENDING, INSERTING, INSERTED
		return store.StatusRunning
	}
}

// refreshBatch synchronise une campagne batch avec l'état OVH (au plus toutes les 3 s).
func (s *Service) refreshBatch(ctx context.Context, c store.Campaign, force bool) store.Campaign {
	if c.Mode != store.ModeBatch || c.BatchID == "" || !store.Active(c.Status) {
		return c
	}
	if !force && !s.batches.due(c.ID, s.Now()) {
		return c
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	acc, sec, err := s.Creds.Load(c.AccountID)
	if err != nil {
		return c
	}
	cl, err := s.NewClient(toOVH(acc, sec))
	if err != nil {
		return c
	}
	b, err := cl.GetBatch(ctx, acc.ServiceName, c.BatchID)
	if err != nil {
		return c // état précédent conservé
	}
	st, err := cl.BatchStats(ctx, acc.ServiceName, c.BatchID)
	if err != nil {
		return c
	}
	credits := st.Credits
	if credits == 0 {
		credits = b.EstimatedCredits
	}
	var msgs []string
	for i, e := range b.Errors {
		if i == 3 {
			msgs = append(msgs, fmt.Sprintf("… et %d autre(s)", len(b.Errors)-3))
			break
		}
		msgs = append(msgs, e.Receiver+" : "+e.Message)
	}
	errMsg := strings.Join(msgs, " ; ")
	status := batchStatus(b.Status)
	// Les compteurs OVH se recouvrent (un SMS délivré compte aussi comme envoyé) : on prend le plus grand,
	// et on ne dépasse jamais le total.
	sent := int(max(st.Delivered, st.Sent) + st.Stoplisted)
	if t := int(b.TotalRecords); t > 0 {
		sent = min(sent, max(t-int(st.Failed), 0))
	}
	if err := s.Store.SetProgress(ctx, c.ID, status, int(b.TotalRecords), sent, int(st.Failed), credits, errMsg); err != nil {
		return c
	}
	if fresh, err := s.Store.GetCampaign(ctx, c.ID); err == nil {
		return fresh
	}
	return c
}

// ListCampaigns renvoie les campagnes (les batches actifs sont resynchronisés avec OVH).
func (s *Service) ListCampaigns(ctx context.Context) ([]store.Campaign, error) {
	cs, err := s.Store.ListCampaigns(ctx)
	for i := range cs {
		cs[i] = s.refreshBatch(ctx, cs[i], false)
	}
	return cs, err
}

// GetCampaign renvoie une campagne à jour.
func (s *Service) GetCampaign(ctx context.Context, id string) (store.Campaign, error) {
	c, err := s.Store.GetCampaign(ctx, id)
	if err != nil {
		return c, err
	}
	return s.refreshBatch(ctx, c, false), nil
}

// RecipientsPage est une page de destinataires d'une campagne locale.
type RecipientsPage struct {
	Items []store.Recipient `json:"items"`
	Total int               `json:"total"`
}

// CampaignRecipients pagine les destinataires (status : "", "pending", "sent", "failed").
func (s *Service) CampaignRecipients(ctx context.Context, id, status string, offset, limit int) (RecipientsPage, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	items, total, err := s.Store.ListRecipients(ctx, id, status, max(offset, 0), limit)
	return RecipientsPage{Items: items, Total: total}, err
}

func (s *Service) PauseCampaign(ctx context.Context, id string) error {
	c, err := s.Store.GetCampaign(ctx, id)
	if err != nil {
		return err
	}
	if c.Mode == store.ModeBatch {
		return invalid("%s", i18n.T("svc.batch_no_pause"))
	}
	return s.Runner.Pause(id)
}

func (s *Service) ResumeCampaign(ctx context.Context, id string) error {
	return s.Runner.Start(id)
}

func (s *Service) CancelCampaign(ctx context.Context, id string) error {
	c, err := s.Store.GetCampaign(ctx, id)
	if err != nil {
		return err
	}
	if c.Mode == store.ModeLocal {
		return s.Runner.Cancel(id)
	}
	acc, sec, err := s.Creds.Load(c.AccountID)
	if err != nil {
		return err
	}
	cl, err := s.NewClient(toOVH(acc, sec))
	if err != nil {
		return err
	}
	if _, err := cl.CancelBatch(ctx, acc.ServiceName, c.BatchID); err != nil {
		return err
	}
	s.refreshBatch(ctx, c, true)
	return nil
}

// RetryFailed relance les destinataires en échec d'une campagne locale.
func (s *Service) RetryFailed(ctx context.Context, id string) (int, error) {
	c, err := s.Store.GetCampaign(ctx, id)
	if err != nil {
		return 0, err
	}
	if c.Mode != store.ModeLocal {
		return 0, invalid("%s", i18n.T("svc.retry_only_custom"))
	}
	if s.Runner.Running(id) || c.Status == store.StatusCanceled {
		return 0, invalid("%s", i18n.T("svc.pause_first"))
	}
	n, err := s.Store.RetryFailed(ctx, id)
	if err != nil || n == 0 {
		return n, err
	}
	if err := s.Store.SetStatus(ctx, id, store.StatusPaused, ""); err != nil {
		return n, err
	}
	return n, s.Runner.Start(id)
}

// DeleteCampaign supprime une campagne de l'historique local (rien n'est supprimé chez OVH).
func (s *Service) DeleteCampaign(ctx context.Context, id string) error {
	c, err := s.Store.GetCampaign(ctx, id)
	if err != nil {
		return err
	}
	if s.Runner.Running(id) || (c.Mode == store.ModeBatch && store.Active(c.Status)) {
		return invalid("%s", i18n.T("svc.campaign_running"))
	}
	return s.Store.DeleteCampaign(ctx, id)
}
