package ovh

import (
	"context"
	"regexp"
	"sync"
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
	for range res.ValidReceivers {
		m.nextID++
		res.IDs = append(res.IDs, m.nextID)
	}
	m.Sent = append(m.Sent, req)
	return res, nil
}
