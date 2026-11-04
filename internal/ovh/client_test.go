package ovh

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	goovh "github.com/ovh/go-ovh/ovh"
)

type fakeCaller struct {
	gets  map[string]string // url -> JSON
	err   error
	posts []struct {
		url  string
		body jobBody
	}
	postRes  string
	lastURL  string
	lastBody interface{}
}

func (f *fakeCaller) GetWithContext(_ context.Context, url string, res interface{}) error {
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(f.gets[url]), res)
}

func (f *fakeCaller) GetUnAuthWithContext(ctx context.Context, url string, res interface{}) error {
	return f.GetWithContext(ctx, url, res)
}

func (f *fakeCaller) PostWithContext(_ context.Context, url string, body, res interface{}) error {
	if f.err != nil {
		return f.err
	}
	if jb, ok := body.(jobBody); ok {
		f.posts = append(f.posts, struct {
			url  string
			body jobBody
		}{url, jb})
	} else {
		f.lastURL, f.lastBody = url, body
	}
	return json.Unmarshal([]byte(f.postRes), res)
}

func newTestClient(f *fakeCaller) *client {
	return &client{api: f, now: func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }}
}

func TestAccountAndSenders(t *testing.T) {
	f := &fakeCaller{gets: map[string]string{
		"/sms/sms-ab1":              `{"name":"sms-ab1","status":"enable","creditsLeft":42.5}`,
		"/sms/sms-ab1/senders":      `["Smsh","Old"]`,
		"/sms/sms-ab1/senders/Smsh": `{"status":"enable"}`,
		"/sms/sms-ab1/senders/Old":  `{"status":"disable"}`,
	}}
	c := newTestClient(f)
	acc, err := c.Account(context.Background(), "sms-ab1")
	if err != nil || acc.CreditsLeft != 42.5 || acc.Status != "enable" {
		t.Fatalf("account = %+v, %v", acc, err)
	}
	s, err := c.Senders(context.Background(), "sms-ab1")
	if err != nil || len(s) != 2 || s[0].Name != "Smsh" || s[1].Status != "disable" {
		t.Fatalf("senders = %+v, %v", s, err)
	}
}

func TestSendBuildsBody(t *testing.T) {
	f := &fakeCaller{postRes: `{"ids":[7],"validReceivers":["+33612345678"],"invalidReceivers":[],"totalCreditsRemoved":1}`}
	c := newTestClient(f)
	at := c.now().Add(90*time.Second + time.Minute) // 150 s -> 3 min
	res, err := c.Send(context.Background(), SendRequest{
		Service: "sms-ab1", Message: "Salut", Receivers: []string{"+33612345678"},
		Sender: "Smsh", SendAt: &at, Unicode: true,
	})
	if err != nil || res.IDs[0] != 7 || res.CreditsRemoved != 1 {
		t.Fatalf("res = %+v, %v", res, err)
	}
	p := f.posts[0]
	if p.url != "/sms/sms-ab1/jobs" {
		t.Errorf("url = %s", p.url)
	}
	b := p.body
	if b.Sender != "Smsh" || b.SenderForResponse || !b.NoStopClause || b.Coding != "8bit" || b.DifferedPeriod != 3 {
		t.Errorf("body = %+v", b)
	}
}

func TestSendDefaultSenderAndValidation(t *testing.T) {
	f := &fakeCaller{postRes: `{}`}
	c := newTestClient(f)
	if _, err := c.Send(context.Background(), SendRequest{Service: "s", Message: "x", Receivers: []string{"+33612345678"}}); err != nil {
		t.Fatal(err)
	}
	if !f.posts[0].body.SenderForResponse || f.posts[0].body.Coding != "7bit" {
		t.Errorf("body = %+v", f.posts[0].body)
	}
	past := c.now().Add(-time.Hour)
	_, err := c.Send(context.Background(), SendRequest{Service: "s", Message: "x", Receivers: []string{"+33612345678"}, SendAt: &past})
	if KindOf(err) != KindInvalidRequest {
		t.Errorf("past date: %v", err)
	}
	_, err = c.Send(context.Background(), SendRequest{Service: "s"})
	if KindOf(err) != KindInvalidRequest {
		t.Errorf("empty: %v", err)
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Kind
	}{
		{"429", &goovh.APIError{Code: 429, Message: "slow down"}, KindRateLimit},
		{"401", &goovh.APIError{Code: 401, Message: "bad"}, KindCredentials},
		{"403", &goovh.APIError{Code: 403, Message: "This call has not been granted"}, KindCredentials},
		{"404", &goovh.APIError{Code: 404, Message: "not found"}, KindNotFound},
		{"credits", &goovh.APIError{Code: 400, Message: "Not enough credit"}, KindInsufficientCredits},
		{"receiver", &goovh.APIError{Code: 400, Message: "Invalid receiver"}, KindInvalidReceiver},
		{"400", &goovh.APIError{Code: 400, Message: "weird"}, KindInvalidRequest},
		{"500", &goovh.APIError{Code: 500, Message: "boom"}, KindUnknown},
		{"timeout", context.DeadlineExceeded, KindNetwork},
		{"net", &net.DNSError{Err: "no such host", IsTemporary: true}, KindNetwork},
		{"wrapped", errors.Join(errors.New("x"), &goovh.APIError{Code: 429}), KindRateLimit},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeCaller{err: tc.err}
			_, err := newTestClient(f).Account(context.Background(), "s")
			if got := KindOf(err); got != tc.want {
				t.Fatalf("kind = %s, want %s (err=%v)", got, tc.want, err)
			}
		})
	}
}

func TestMockInsufficientCredits(t *testing.T) {
	m := NewMock()
	m.Info.CreditsLeft = 1
	_, err := m.Send(context.Background(), SendRequest{Receivers: []string{"+33611111111", "+33622222222"}})
	if KindOf(err) != KindInsufficientCredits {
		t.Fatalf("err = %v", err)
	}
	res, err := m.Send(context.Background(), SendRequest{Receivers: []string{"+33611111111", "abc"}})
	if err != nil || len(res.InvalidReceivers) != 1 || m.Info.CreditsLeft != 0 {
		t.Fatalf("res = %+v, err = %v, credits = %v", res, err, m.Info.CreditsLeft)
	}
}

func TestEstimateAndRates(t *testing.T) {
	f := &fakeCaller{
		postRes: `{"characters":9,"charactersClass":"7bits","maxCharactersPerPart":160,"parts":1}`,
		gets: map[string]string{
			"/sms/rates/destinations?billingCountry=fr&country=us": `{"credit":0.35,"price":{"currencyCode":"EUR","value":0.021}}`,
			"/sms/rates/packs?billingCountry=fr&country=us":        `[{"creditQuantityMin":500,"smsPrice":{"value":0.02}},{"creditQuantityMin":100,"smsPrice":{"value":0.021}}]`,
		},
	}
	c := newTestClient(f)
	me, err := c.EstimateMessage(context.Background(), "Bonjour !", false, "alpha")
	if err != nil || me.Parts != 1 || me.Encoding != "7bits" || me.PerPart != 160 || f.lastURL != "/sms/estimate" {
		t.Fatalf("me=%+v err=%v url=%s", me, err, f.lastURL)
	}
	b := f.lastBody.(map[string]interface{})
	if b["noStopClause"] != true || b["senderType"] != "alpha" {
		t.Errorf("body = %v", b)
	}
	r, err := c.DestinationRate(context.Background(), "fr", "us")
	if err != nil || r.CreditsPerSMS != 0.35 || r.Currency != "EUR" {
		t.Fatalf("rate=%+v err=%v", r, err)
	}
	tiers, err := c.PackTiers(context.Background(), "fr", "us")
	if err != nil || len(tiers) != 2 || tiers[0].CreditsMin != 500 {
		t.Fatalf("tiers=%+v err=%v", tiers, err)
	}
}

func TestBatchCalls(t *testing.T) {
	f := &fakeCaller{
		postRes: `{"id":"b-1","status":"PENDING","totalRecords":2,"estimatedCredits":2}`,
		gets: map[string]string{
			"/sms/sms-ab1/batches/b-1":            `{"id":"b-1","status":"COMPLETED","totalRecords":2,"processedRecords":2}`,
			"/sms/sms-ab1/batches/b-1/statistics": `{"credits":2,"delivered":1,"failed":1,"pending":0,"sent":0,"stoplisted":0}`,
		},
	}
	c := newTestClient(f)
	at := c.now().Add(time.Hour)
	b, err := c.CreateBatch(context.Background(), BatchRequest{
		Service: "sms-ab1", Name: "Promo", Message: "Salut", To: []string{"+33612345678", "+33698765432"}, SendAt: &at, Tag: "t1",
	})
	if err != nil || b.ID != "b-1" || f.lastURL != "/sms/sms-ab1/batches" {
		t.Fatalf("b=%+v err=%v url=%s", b, err, f.lastURL)
	}
	body := f.lastBody.(batchBody)
	if !body.SenderForResponse || !body.NoStop || body.Deferred != "2026-01-01T13:00:00Z" || len(body.To) != 2 {
		t.Errorf("body = %+v", body)
	}
	got, err := c.GetBatch(context.Background(), "sms-ab1", "b-1")
	if err != nil || got.Status != "COMPLETED" || got.ProcessedRecords != 2 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	st, err := c.BatchStats(context.Background(), "sms-ab1", "b-1")
	if err != nil || st.Delivered != 1 || st.Failed != 1 || st.Credits != 2 {
		t.Fatalf("st=%+v err=%v", st, err)
	}
	if _, err := c.CancelBatch(context.Background(), "sms-ab1", "b-1"); err != nil || f.lastURL != "/sms/sms-ab1/batches/b-1/cancel" {
		t.Fatalf("cancel err=%v url=%s", err, f.lastURL)
	}
	past := c.now().Add(-time.Hour)
	if _, err := c.CreateBatch(context.Background(), BatchRequest{Service: "s", Name: "n", Message: "m", To: []string{"+33612345678"}, SendAt: &past}); KindOf(err) != KindInvalidRequest {
		t.Errorf("date passée : %v", err)
	}
}
