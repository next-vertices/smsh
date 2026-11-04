package ovh

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"
)

var e164 = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)

// Mock est un Client en mémoire pour les tests et le mode démo (SMSH_MOCK=1).
type Mock struct {
	mu       sync.Mutex
	Services []string
	Info     AccountInfo
	List     []Sender
	// Err, si défini, est renvoyé par chaque appel (simule credentials expirés, etc.).
	Err error
	// CreditsPerReceiver : coût par destinataire valide (1 par défaut).
	CreditsPerReceiver float64
	Sent               []SendRequest
	Batches            map[string]Batch
	Outbox             map[int64]Outgoing
	MaxList            int // si > 0 : ListOutgoing échoue au-delà (comme OVH)
	nextID             int64
}

// NewMock renvoie un Mock avec des données plausibles.
func NewMock() *Mock {
	return &Mock{
		Services: []string{"sms-demo-1"},
		Info:     AccountInfo{Name: "sms-demo-1", Status: "enable", CreditsLeft: 100},
		List:     []Sender{{Name: "Smsh", Status: "enable"}, {Name: "Perso", Status: "disable"}},
	}
}

func (m *Mock) ListServices(context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Services, nil
}

func (m *Mock) Account(context.Context, string) (AccountInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return AccountInfo{}, m.Err
	}
	return m.Info, nil
}

func (m *Mock) Senders(context.Context, string) ([]Sender, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	return m.List, nil
}

func (m *Mock) EstimateMessage(_ context.Context, message string, _ bool, _ string) (MessageEstimate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return MessageEstimate{}, m.Err
	}
	n := len([]rune(message))
	return MessageEstimate{Characters: n, Encoding: "7bits", PerPart: 160, Parts: (n + 152) / 153}, nil
}

// DestinationRate : fictif — 1 crédit en France, 0,35 aux États-Unis, 2 ailleurs.
func (m *Mock) DestinationRate(_ context.Context, _, country string) (DestinationRate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return DestinationRate{}, m.Err
	}
	credits := map[string]float64{"fr": 1, "us": 0.35}[country]
	if credits == 0 {
		credits = 2
	}
	return DestinationRate{Country: country, CreditsPerSMS: credits, PricePerSMS: 0.06 * credits, Currency: "EUR"}, nil
}

func (m *Mock) PackTiers(context.Context, string, string) ([]PackTier, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	return []PackTier{{CreditsMin: 100, PricePerSMS: 0.06}, {CreditsMin: 5000, PricePerSMS: 0.05}}, nil
}

// ---- historique fictif ----

func (m *Mock) ListOutgoing(_ context.Context, _ string, f OutgoingFilter) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	ids := []int64{}
	for id, o := range m.Outbox {
		if (f.From != nil && o.CreatedAt.Before(*f.From)) || (f.To != nil && o.CreatedAt.After(*f.To)) ||
			(f.Receiver != "" && o.Receiver != f.Receiver) || (f.Tag != "" && o.Tag != f.Tag) {
			continue
		}
		ids = append(ids, id)
	}
	if m.MaxList > 0 && len(ids) > m.MaxList {
		return nil, &Error{Kind: KindTooManyResults, Status: 400, Message: "Trop de résultats : OVH demande de restreindre la période."}
	}
	return ids, nil
}

func (m *Mock) GetOutgoing(_ context.Context, _ string, id int64) (Outgoing, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return Outgoing{}, m.Err
	}
	o, ok := m.Outbox[id]
	if !ok {
		return Outgoing{}, &Error{Kind: KindNotFound, Message: "SMS introuvable.", Status: 404}
	}
	return o, nil
}

func (m *Mock) DeleteOutgoing(_ context.Context, _ string, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if _, ok := m.Outbox[id]; !ok {
		return &Error{Kind: KindNotFound, Message: "SMS introuvable.", Status: 404}
	}
	delete(m.Outbox, id)
	return nil
}

// ---- batches fictifs : se terminent dès le premier GetBatch ----

func (m *Mock) CreateBatch(_ context.Context, req BatchRequest) (Batch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return Batch{}, m.Err
	}
	cost := float64(len(req.To))
	if cost > m.Info.CreditsLeft {
		return Batch{}, &Error{Kind: KindInsufficientCredits, Message: "Crédits SMS insuffisants pour cet envoi.", Status: 400}
	}
	m.nextID++
	b := Batch{ID: fmt.Sprintf("batch-%d", m.nextID), Name: req.Name, Status: "PENDING",
		TotalRecords: int64(len(req.To)), EstimatedCredits: cost}
	if m.Batches == nil {
		m.Batches = map[string]Batch{}
	}
	m.Batches[b.ID] = b
	return b, nil
}

func (m *Mock) GetBatch(_ context.Context, _, id string) (Batch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return Batch{}, m.Err
	}
	b, ok := m.Batches[id]
	if !ok {
		return Batch{}, &Error{Kind: KindNotFound, Message: "Batch introuvable."}
	}
	if b.Status == "PENDING" || b.Status == "INSERTING" {
		b.Status, b.ProcessedRecords = "COMPLETED", b.TotalRecords
		m.Batches[id] = b
	}
	return b, nil
}

func (m *Mock) BatchStats(_ context.Context, _, id string) (BatchStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return BatchStats{}, m.Err
	}
	b, ok := m.Batches[id]
	if !ok {
		return BatchStats{}, &Error{Kind: KindNotFound, Message: "Batch introuvable."}
	}
	st := BatchStats{EstimatedCredits: b.EstimatedCredits}
	if b.Status == "COMPLETED" {
		st.Credits, st.Delivered = b.EstimatedCredits, b.TotalRecords
	} else {
		st.Pending = b.TotalRecords
	}
	return st, nil
}

func (m *Mock) CancelBatch(_ context.Context, _, id string) (Batch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.Batches[id]
	if !ok {
		return Batch{}, &Error{Kind: KindNotFound, Message: "Batch introuvable."}
	}
	b.Status = "CANCELED"
	m.Batches[id] = b
	return b, nil
}

func (m *Mock) Send(_ context.Context, req SendRequest) (SendResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return SendResult{}, m.Err
	}
	var res SendResult
	for _, r := range req.Receivers {
		if e164.MatchString(r) {
			res.ValidReceivers = append(res.ValidReceivers, r)
		} else {
			res.InvalidReceivers = append(res.InvalidReceivers, r)
		}
	}
	per := m.CreditsPerReceiver
	if per == 0 {
		per = 1
	}
	cost := per * float64(len(res.ValidReceivers))
	if cost > m.Info.CreditsLeft {
		return SendResult{}, &Error{Kind: KindInsufficientCredits, Message: "Crédits SMS insuffisants pour cet envoi.", Status: 400}
	}
	m.Info.CreditsLeft -= cost
	res.CreditsRemoved = cost
	if m.Outbox == nil {
		m.Outbox = map[int64]Outgoing{}
	}
	now := time.Now()
	for _, r := range res.ValidReceivers {
		m.nextID++
		res.IDs = append(res.IDs, m.nextID)
		o := Outgoing{ID: m.nextID, CreatedAt: now, SentAt: &now, Receiver: r, Sender: req.Sender,
			Message: req.Message, Credits: per, Parts: 1, Tag: req.Tag}
		if m.nextID%2 == 1 { // la moitié des SMS reçoit un accusé de réception
			d := now.Add(2 * time.Second)
			o.DeliveredAt = &d
		}
		m.Outbox[o.ID] = o
	}
	m.Sent = append(m.Sent, req)
	return res, nil
}
