package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"smsh/internal/ovh"
)

func seedHistory(t *testing.T, s *Service) {
	t.Helper()
	ctx := context.Background()
	for _, in := range []SendInput{
		{Message: "Bonjour Léa", Recipients: "0611111111, 0622222222", Sender: "Smsh"},
		{Message: "Promo 100% sûr", Recipients: "0633333333"},
	} {
		if _, err := s.Send(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
}

func search(t *testing.T, s *Service, q HistoryQuery) int {
	t.Helper()
	p, err := s.SearchHistory(context.Background(), q, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	return p.Total
}

func TestSyncAndSearchHistory(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	seedHistory(t, s)

	res, err := s.SyncHistory(ctx, 7)
	if err != nil || res.Listed != 3 || res.Fetched != 3 || res.Remaining != 0 || res.Removed != 0 {
		t.Fatalf("sync = %+v err=%v", res, err)
	}
	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	cases := []struct {
		name string
		q    HistoryQuery
		want int
	}{
		{"tout", HistoryQuery{}, 3},
		{"texte insensible à la casse", HistoryQuery{Query: "promo"}, 1},
		{"% littéral échappé", HistoryQuery{Query: "100%"}, 1},
		{"% seul ne matche pas tout", HistoryQuery{Query: "%"}, 1},
		{"numéro national normalisé", HistoryQuery{Receiver: "06 11 11 11 11"}, 1},
		{"fragment de numéro", HistoryQuery{Receiver: "2222"}, 1},
		{"expéditeur", HistoryQuery{Sender: "Smsh"}, 2},
		{"statut délivré", HistoryQuery{Status: "delivered"}, 2}, // ids impairs du mock
		{"statut envoyé", HistoryQuery{Status: "sent"}, 1},
		{"statut différé", HistoryQuery{Status: "scheduled"}, 0},
		{"aujourd'hui", HistoryQuery{DateFrom: today, DateTo: today}, 3},
		{"à partir de demain", HistoryQuery{DateFrom: tomorrow}, 0},
		{"combiné", HistoryQuery{Sender: "Smsh", Status: "delivered"}, 1},
	}
	for _, c := range cases {
		if got := search(t, s, c.q); got != c.want {
			t.Errorf("%s : %d résultat(s), attendu %d", c.name, got, c.want)
		}
	}
	if _, err := s.SearchHistory(ctx, HistoryQuery{DateFrom: "hier"}, 0, 10); err == nil {
		t.Error("date invalide : erreur attendue")
	}
	if _, err := s.SearchHistory(ctx, HistoryQuery{DateFrom: tomorrow, DateTo: today}, 0, 10); err == nil {
		t.Error("période inversée : erreur attendue")
	}
	p, _ := s.SearchHistory(ctx, HistoryQuery{}, 0, 2)
	if len(p.Items) != 2 || p.Total != 3 || p.Credits != 3 || p.Items[0].ID < p.Items[1].ID {
		t.Errorf("pagination/tri : %+v", p)
	}
	f, _ := s.HistoryFacets(ctx)
	if len(f.Senders) != 1 || f.Senders[0] != "Smsh" {
		t.Errorf("facettes = %+v", f)
	}
}

func TestSyncRefreshesUndeliveredAndRemovesGone(t *testing.T) {
	s, mock := newTestService(t)
	ctx := context.Background()
	seedHistory(t, s)
	if _, err := s.SyncHistory(ctx, 7); err != nil {
		t.Fatal(err)
	}
	// Seconde synchro : rien de nouveau ; seul le SMS sans accusé (id 2) est ré-interrogé.
	res, err := s.SyncHistory(ctx, 7)
	if err != nil || res.Fetched != 1 || res.Remaining != 0 {
		t.Fatalf("resync = %+v err=%v", res, err)
	}
	// Un SMS supprimé directement chez OVH disparaît du cache.
	_ = mock.DeleteOutgoing(ctx, "", 3)
	res, err = s.SyncHistory(ctx, 7)
	if err != nil || res.Removed != 1 || res.Listed != 2 {
		t.Fatalf("après suppression distante = %+v err=%v", res, err)
	}
	if n := search(t, s, HistoryQuery{}); n != 2 {
		t.Errorf("cache = %d, attendu 2", n)
	}
}

func TestSyncStopsOnRateLimit(t *testing.T) {
	s, mock := newTestService(t)
	seedHistory(t, s)
	// La liste passe, mais le téléchargement des détails est refusé.
	mock.Err = nil
	cl := &failingDetails{Mock: mock}
	s.NewClient = func(ovh.Credentials) (ovh.Client, error) { return cl, nil }
	res, err := s.SyncHistory(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if res.Fetched != 0 || res.Remaining != 3 || len(res.Errors) == 0 {
		t.Fatalf("res = %+v", res)
	}
}

type failingDetails struct{ *ovh.Mock }

func (f *failingDetails) GetOutgoing(context.Context, string, int64) (ovh.Outgoing, error) {
	return ovh.Outgoing{}, &ovh.Error{Kind: ovh.KindRateLimit, Message: "Trop de requêtes"}
}

func TestHistoryDetailAndDelete(t *testing.T) {
	s, mock := newTestService(t)
	ctx := context.Background()
	seedHistory(t, s)
	if _, err := s.SyncHistory(ctx, 7); err != nil {
		t.Fatal(err)
	}
	d, err := s.HistoryDetail(ctx, 2)
	if err != nil || d.Receiver != "+33622222222" || d.Status != "sent" || d.Message != "Bonjour Léa" {
		t.Fatalf("detail = %+v err=%v", d, err)
	}

	ids, err := s.HistoryMatchingIDs(ctx, HistoryQuery{Sender: "Smsh"})
	if err != nil || len(ids) != 2 {
		t.Fatalf("ids = %v err=%v", ids, err)
	}
	_ = mock.DeleteOutgoing(ctx, "", ids[0]) // déjà absent chez OVH : compte comme supprimé
	res, err := s.DeleteHistory(ctx, ids)
	if err != nil || res.Deleted != 2 || len(res.Failed) != 0 {
		t.Fatalf("delete = %+v err=%v", res, err)
	}
	if n := search(t, s, HistoryQuery{}); n != 1 {
		t.Errorf("reste %d SMS en cache, attendu 1", n)
	}
	if len(mock.Outbox) != 1 {
		t.Errorf("reste %d SMS chez OVH, attendu 1", len(mock.Outbox))
	}
	if _, err := s.DeleteHistory(ctx, make([]int64, maxDeleteBatch+1)); err == nil {
		t.Error("lot trop grand : erreur attendue")
	}
}

func TestDeleteHistoryReportsFailures(t *testing.T) {
	s, mock := newTestService(t)
	seedHistory(t, s)
	ctx := context.Background()
	_, _ = s.SyncHistory(ctx, 7)
	mock.Err = &ovh.Error{Kind: ovh.KindCredentials, Message: "refusé"}
	res, err := s.DeleteHistory(ctx, []int64{1, 2, 3})
	if err != nil || res.Deleted != 0 || len(res.Failed) != 3 {
		t.Fatalf("res = %+v err=%v", res, err)
	}
	mock.Err = nil
	if n := search(t, s, HistoryQuery{}); n != 3 {
		t.Errorf("rien ne doit quitter le cache en cas d'échec : %d", n)
	}
}

func TestSyncSplitsPeriodWhenOVHRefusesLongList(t *testing.T) {
	s, mock := newTestService(t)
	ctx := context.Background()
	// 12 SMS étalés sur 12 jours ; OVH refuse toute liste de plus de 3 résultats.
	mock.Outbox = map[int64]ovh.Outgoing{}
	for i := int64(1); i <= 12; i++ {
		at := time.Now().Add(-time.Duration(i) * 24 * time.Hour)
		mock.Outbox[i] = ovh.Outgoing{ID: i, CreatedAt: at, SentAt: &at, Receiver: "+33611111111", Message: "x", Credits: 1, Parts: 1}
	}
	mock.MaxList = 3
	res, err := s.SyncHistory(ctx, 30)
	if err != nil || res.Listed != 12 || res.Fetched != 12 {
		t.Fatalf("res = %+v err=%v", res, err)
	}
	if n := search(t, s, HistoryQuery{}); n != 12 {
		t.Errorf("cache = %d, attendu 12", n)
	}
	// Tous les SMS au même instant : impossible à découper, l'erreur doit remonter proprement.
	now := time.Now()
	mock.Outbox = map[int64]ovh.Outgoing{}
	for i := int64(100); i < 106; i++ {
		mock.Outbox[i] = ovh.Outgoing{ID: i, CreatedAt: now, SentAt: &now, Receiver: "+33611111111", Message: "x", Parts: 1}
	}
	if _, err := s.SyncHistory(ctx, 30); ovh.KindOf(err) != ovh.KindTooManyResults {
		t.Errorf("erreur attendue (non découpable), obtenu : %v", err)
	}
}

type flaky struct {
	*ovh.Mock
	mu    sync.Mutex
	fails int
	calls int
}

func (f *flaky) ListOutgoing(ctx context.Context, svc string, fl ovh.OutgoingFilter) ([]int64, error) {
	f.mu.Lock()
	f.calls++
	fail := f.calls <= f.fails
	f.mu.Unlock()
	if fail {
		return nil, &ovh.Error{Kind: ovh.KindUnknown, Status: 500, Message: "Internal server error"}
	}
	return f.Mock.ListOutgoing(ctx, svc, fl)
}

func TestSyncRetriesTransientErrorsAndFetchesNewestFirst(t *testing.T) {
	old := retryDelays
	retryDelays = []time.Duration{0, 0}
	defer func() { retryDelays = old }()

	s, mock := newTestService(t)
	seedHistory(t, s)
	fl := &flaky{Mock: mock, fails: 2}
	s.NewClient = func(ovh.Credentials) (ovh.Client, error) { return fl, nil }
	res, err := s.SyncHistory(context.Background(), 7)
	if err != nil || res.Fetched != 3 {
		t.Fatalf("res = %+v err=%v", res, err)
	}
	fl.calls, fl.fails = 0, 99 // plus de tentatives que prévu : l'erreur remonte
	if _, err := s.SyncHistory(context.Background(), 7); err == nil {
		t.Error("erreur attendue après épuisement des tentatives")
	}
}

func TestOnlyOneSyncAtATime(t *testing.T) {
	s, _ := newTestService(t)
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if _, err := s.SyncHistory(context.Background(), 7); err == nil {
		t.Error("une synchronisation concurrente doit être refusée")
	}
}
