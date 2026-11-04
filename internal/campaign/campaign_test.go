package campaign

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"

	"smsh/internal/ovh"
	"smsh/internal/store"
)

func TestParseCSV(t *testing.T) {
	tb, err := ParseCSV([]byte("\xEF\xBB\xBFNom;Téléphone;Prénom\nDupont;06 12 34 56 78;Jean\n\nMartin;0698765432;Léa\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tb.Headers) != 3 || len(tb.Rows) != 2 || tb.Headers[1] != "Téléphone" {
		t.Fatalf("tb = %+v", tb)
	}
	if c := DetectPhoneColumn(tb); c != 1 {
		t.Errorf("colonne détectée = %d", c)
	}
}

func TestParseCSVWindows1252AndDelimiters(t *testing.T) {
	enc, _ := charmap.Windows1252.NewEncoder().String("prénom,mobile\nÉlodie,0612345678\n")
	tb, err := ParseCSV([]byte(enc))
	if err != nil || tb.Headers[0] != "prénom" || tb.Rows[0][0] != "Élodie" {
		t.Fatalf("tb=%+v err=%v", tb, err)
	}
	tb, err = ParseCSV([]byte("a\tb\n1\t2\n"))
	if err != nil || len(tb.Headers) != 2 {
		t.Fatalf("tab : %+v %v", tb, err)
	}
	if _, err := ParseCSV([]byte("seulement,entetes\n")); err == nil {
		t.Error("erreur attendue sans données")
	}
}

func TestDetectPhoneColumnByContent(t *testing.T) {
	tb, _ := ParseCSV([]byte("a,b\nx,0612345678\ny,0698765432\n"))
	if c := DetectPhoneColumn(tb); c != 1 {
		t.Fatalf("col = %d", c)
	}
}

func TestExtractContacts(t *testing.T) {
	tb, _ := ParseCSV([]byte("nom;tel\nA;0612345678\nB;+33612345678\nC;abc\nD;\nE;0698765432\n"))
	c, err := ExtractContacts(tb, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Valid) != 2 || c.Dupes != 1 || len(c.Invalid) != 2 {
		t.Fatalf("c = %+v", c)
	}
	if c.Invalid[0].Line != 4 || c.Valid[1].Vars["nom"] != "E" {
		t.Errorf("c = %+v", c)
	}
	if _, err := ExtractContacts(tb, 9); err == nil {
		t.Error("colonne invalide : erreur attendue")
	}
}

func TestTemplate(t *testing.T) {
	got, err := Render("Bonjour {Prénom}, rdv le {date}.", map[string]string{"prénom": "Léa", "Date": "lundi"})
	if err != nil || got != "Bonjour Léa, rdv le lundi." {
		t.Fatalf("got %q err %v", got, err)
	}
	if _, err := Render("Hello {x} {y}", map[string]string{"x": "1"}); err == nil {
		t.Error("variable inconnue : erreur attendue")
	}
	if v := Variables("{a} {A} {b}"); len(v) != 2 {
		t.Errorf("variables = %v", v)
	}
	if u := Unknown("{a} {zz}", []string{"A"}); len(u) != 1 || u[0] != "zz" {
		t.Errorf("unknown = %v", u)
	}
}

// ---- moteur ----

type fakeSend struct {
	mu    sync.Mutex
	calls []string
	fn    func(n int, r store.Recipient) (ovh.SendResult, error)
}

func (f *fakeSend) send(_ context.Context, _ store.Campaign, r store.Recipient) (ovh.SendResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, r.Phone)
	n := len(f.calls)
	f.mu.Unlock()
	if f.fn != nil {
		return f.fn(n, r)
	}
	return ovh.SendResult{IDs: []int64{int64(n)}, ValidReceivers: []string{r.Phone}, CreditsRemoved: 1}, nil
}

func setup(t *testing.T, n int, fs *fakeSend) (*Runner, *store.Store, string, *[]time.Duration) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	rs := make([]store.Recipient, n)
	for i := range rs {
		rs[i] = store.Recipient{Idx: i, Phone: fmt.Sprintf("+3361234%04d", i), Message: "hi", Vars: map[string]string{}}
	}
	id := "c1"
	err = st.CreateCampaign(context.Background(), store.Campaign{
		ID: id, Name: "T", Mode: store.ModeLocal, AccountID: "a", Template: "hi", Rate: 2, Status: store.StatusPending, Total: n,
	}, rs)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner(st, fs.send)
	var sleeps []time.Duration
	var mu sync.Mutex
	r.Sleep = func(ctx context.Context, d time.Duration) error {
		mu.Lock()
		sleeps = append(sleeps, d)
		mu.Unlock()
		return ctx.Err()
	}
	return r, st, id, &sleeps
}

func get(t *testing.T, st *store.Store, id string) store.Campaign {
	t.Helper()
	c, err := st.GetCampaign(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRunnerCompletes(t *testing.T) {
	fs := &fakeSend{}
	r, st, id, sleeps := setup(t, 5, fs)
	if err := r.Start(id); err != nil {
		t.Fatal(err)
	}
	r.Wait(id)
	c := get(t, st, id)
	if c.Status != store.StatusCompleted || c.Sent != 5 || c.Failed != 0 || c.Credits != 5 {
		t.Fatalf("c = %+v", c)
	}
	// Débit 2/s : 4 attentes (entre 5 envois) d'au plus 500 ms.
	if len(*sleeps) != 4 {
		t.Fatalf("sleeps = %v", *sleeps)
	}
	for _, d := range *sleeps {
		if d > 500*time.Millisecond || d <= 0 {
			t.Errorf("attente %v hors limite", d)
		}
	}
}

func TestRunnerPauseResumeKeepsProgress(t *testing.T) {
	var r *Runner
	fs := &fakeSend{}
	var id string
	fs.fn = func(n int, rec store.Recipient) (ovh.SendResult, error) {
		if n == 3 {
			go r.Pause(id) // pause demandée pendant le 3e envoi
			time.Sleep(50 * time.Millisecond)
		}
		return ovh.SendResult{IDs: []int64{int64(n)}, ValidReceivers: []string{rec.Phone}, CreditsRemoved: 1}, nil
	}
	var st *store.Store
	r, st, id, _ = setup(t, 6, fs)
	_ = r.Start(id)
	r.Wait(id)
	c := get(t, st, id)
	if c.Status != store.StatusPaused || c.Sent != 3 {
		t.Fatalf("après pause : %+v", c)
	}
	fs.fn = nil
	if err := r.Start(id); err != nil {
		t.Fatal(err)
	}
	r.Wait(id)
	c = get(t, st, id)
	if c.Status != store.StatusCompleted || c.Sent != 6 || len(fs.calls) != 6 {
		t.Fatalf("après reprise : %+v, appels=%d (aucun doublon attendu)", c, len(fs.calls))
	}
}

func TestRunnerCancel(t *testing.T) {
	var r *Runner
	var id string
	fs := &fakeSend{}
	fs.fn = func(n int, rec store.Recipient) (ovh.SendResult, error) {
		if n == 2 {
			go r.Cancel(id)
			time.Sleep(50 * time.Millisecond)
		}
		return ovh.SendResult{IDs: []int64{1}, ValidReceivers: []string{rec.Phone}, CreditsRemoved: 1}, nil
	}
	var st *store.Store
	r, st, id, _ = setup(t, 5, fs)
	_ = r.Start(id)
	r.Wait(id)
	if c := get(t, st, id); c.Status != store.StatusCanceled || c.Sent != 2 {
		t.Fatalf("c = %+v", c)
	}
	if err := r.Start(id); err == nil {
		t.Error("une campagne annulée ne doit pas redémarrer")
	}
}

func TestRunnerRateLimitRetriesThenSucceeds(t *testing.T) {
	fs := &fakeSend{}
	fails := 0
	fs.fn = func(n int, rec store.Recipient) (ovh.SendResult, error) {
		if fails < 2 {
			fails++
			return ovh.SendResult{}, &ovh.Error{Kind: ovh.KindRateLimit, Message: "429"}
		}
		return ovh.SendResult{IDs: []int64{1}, ValidReceivers: []string{rec.Phone}, CreditsRemoved: 1}, nil
	}
	r, st, id, sleeps := setup(t, 2, fs)
	_ = r.Start(id)
	r.Wait(id)
	c := get(t, st, id)
	if c.Status != store.StatusCompleted || c.Sent != 2 || c.Failed != 0 {
		t.Fatalf("c = %+v", c)
	}
	if (*sleeps)[0] != 2*time.Second || (*sleeps)[1] != 5*time.Second {
		t.Errorf("backoff = %v", *sleeps)
	}
}

func TestRunnerPausesOnInsufficientCredits(t *testing.T) {
	fs := &fakeSend{}
	fs.fn = func(n int, rec store.Recipient) (ovh.SendResult, error) {
		if n > 2 {
			return ovh.SendResult{}, &ovh.Error{Kind: ovh.KindInsufficientCredits, Message: "Crédits SMS insuffisants"}
		}
		return ovh.SendResult{IDs: []int64{1}, ValidReceivers: []string{rec.Phone}, CreditsRemoved: 1}, nil
	}
	r, st, id, _ := setup(t, 5, fs)
	_ = r.Start(id)
	r.Wait(id)
	c := get(t, st, id)
	if c.Status != store.StatusPaused || c.Sent != 2 || c.Failed != 0 || c.Error == "" {
		t.Fatalf("c = %+v", c)
	}
	if len(fs.calls) != 3 {
		t.Errorf("appels = %d, on ne doit pas insister après l'erreur", len(fs.calls))
	}
}

func TestRunnerPausesWhenRetriesExhausted(t *testing.T) {
	fs := &fakeSend{fn: func(int, store.Recipient) (ovh.SendResult, error) {
		return ovh.SendResult{}, &ovh.Error{Kind: ovh.KindRateLimit, Message: "429"}
	}}
	r, st, id, _ := setup(t, 2, fs)
	r.Backoff = []time.Duration{time.Second, time.Second}
	_ = r.Start(id)
	r.Wait(id)
	if c := get(t, st, id); c.Status != store.StatusPaused || c.Sent != 0 || len(fs.calls) != 3 {
		t.Fatalf("c = %+v appels=%d", c, len(fs.calls))
	}
}

func TestRunnerRecipientFailureDoesNotStopCampaign(t *testing.T) {
	fs := &fakeSend{fn: func(n int, rec store.Recipient) (ovh.SendResult, error) {
		switch n {
		case 2:
			return ovh.SendResult{}, &ovh.Error{Kind: ovh.KindInvalidReceiver, Message: "Numéro invalide"}
		case 3:
			return ovh.SendResult{ValidReceivers: nil, InvalidReceivers: []string{rec.Phone}}, nil
		}
		return ovh.SendResult{IDs: []int64{1}, ValidReceivers: []string{rec.Phone}, CreditsRemoved: 1}, nil
	}}
	r, st, id, _ := setup(t, 4, fs)
	_ = r.Start(id)
	r.Wait(id)
	c := get(t, st, id)
	if c.Status != store.StatusCompleted || c.Sent != 2 || c.Failed != 2 {
		t.Fatalf("c = %+v", c)
	}
	failed, total, _ := st.ListRecipients(context.Background(), id, store.RecipientFailed, 0, 10)
	if total != 2 || failed[0].Error != "Numéro invalide" {
		t.Errorf("failed = %+v", failed)
	}
}

func TestResetInterruptedAndRetryFailed(t *testing.T) {
	fs := &fakeSend{fn: func(n int, rec store.Recipient) (ovh.SendResult, error) {
		if n == 1 {
			return ovh.SendResult{}, &ovh.Error{Kind: ovh.KindInvalidRequest, Message: "refusé"}
		}
		return ovh.SendResult{IDs: []int64{1}, ValidReceivers: []string{rec.Phone}, CreditsRemoved: 1}, nil
	}}
	r, st, id, _ := setup(t, 2, fs)
	_ = r.Start(id)
	r.Wait(id)
	n, err := st.RetryFailed(context.Background(), id)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	_ = st.SetStatus(context.Background(), id, store.StatusRunning, "")
	_ = st.ResetInterrupted(context.Background())
	if c := get(t, st, id); c.Status != store.StatusPaused || c.Error == "" {
		t.Fatalf("c = %+v", c)
	}
	_ = r.Start(id)
	r.Wait(id)
	if c := get(t, st, id); c.Status != store.StatusCompleted || c.Sent != 2 || c.Failed != 0 {
		t.Fatalf("c = %+v", c)
	}
}
