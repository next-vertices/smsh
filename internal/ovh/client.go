package ovh

import (
	"context"
	"math"
	"net/url"
	"slices"
	"time"

	goovh "github.com/ovh/go-ovh/ovh"
)

// caller est la partie de *goovh.Client utilisée ici (remplaçable en test).
type caller interface {
	GetWithContext(ctx context.Context, url string, res interface{}) error
	PostWithContext(ctx context.Context, url string, body, res interface{}) error
}

type client struct {
	api caller
	now func() time.Time
}

// New crée un Client branché sur la vraie API OVH.
func New(c Credentials) (Client, error) {
	if !slices.Contains(Endpoints, c.Endpoint) {
		return nil, &Error{Kind: KindInvalidRequest, Message: "Endpoint OVH inconnu : " + c.Endpoint}
	}
	api, err := goovh.NewClient(c.Endpoint, c.AppKey, c.AppSecret, c.ConsumerKey)
	if err != nil {
		return nil, classify(err)
	}
	return &client{api: api, now: time.Now}, nil
}

func svcPath(service string, parts ...string) string {
	p := "/sms/" + url.PathEscape(service)
	for _, s := range parts {
		p += "/" + url.PathEscape(s)
	}
	return p
}

func (c *client) ListServices(ctx context.Context) ([]string, error) {
	var out []string
	if err := c.api.GetWithContext(ctx, "/sms", &out); err != nil {
		return nil, classify(err)
	}
	return out, nil
}

func (c *client) Account(ctx context.Context, service string) (AccountInfo, error) {
	var out AccountInfo
	if err := c.api.GetWithContext(ctx, svcPath(service), &out); err != nil {
		return AccountInfo{}, classify(err)
	}
	return out, nil
}

func (c *client) Senders(ctx context.Context, service string) ([]Sender, error) {
	var names []string
	if err := c.api.GetWithContext(ctx, svcPath(service, "senders"), &names); err != nil {
		return nil, classify(err)
	}
	out := make([]Sender, 0, len(names))
	for _, n := range names {
		var s Sender
		if err := c.api.GetWithContext(ctx, svcPath(service, "senders", n), &s); err != nil {
			return nil, classify(err)
		}
		s.Name = n
		out = append(out, s)
	}
	return out, nil
}

type jobBody struct {
	Message           string   `json:"message"`
	Receivers         []string `json:"receivers"`
	Sender            string   `json:"sender,omitempty"`
	SenderForResponse bool     `json:"senderForResponse,omitempty"`
	NoStopClause      bool     `json:"noStopClause"`
	Coding            string   `json:"coding"`
	DifferedPeriod    int      `json:"differedPeriod,omitempty"`
}

func (c *client) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	if req.Service == "" || req.Message == "" || len(req.Receivers) == 0 {
		return SendResult{}, &Error{Kind: KindInvalidRequest, Message: "Service, message et destinataires sont requis."}
	}
	body := jobBody{
		Message:           req.Message,
		Receivers:         req.Receivers,
		Sender:            req.Sender,
		SenderForResponse: req.Sender == "",
		NoStopClause:      !req.StopClause,
		Coding:            "7bit",
	}
	if req.Unicode {
		body.Coding = "8bit"
	}
	if req.SendAt != nil {
		d := req.SendAt.Sub(c.now())
		if d < 0 {
			return SendResult{}, &Error{Kind: KindInvalidRequest, Message: "La date d'envoi différé est dans le passé."}
		}
		body.DifferedPeriod = int(math.Ceil(d.Minutes()))
	}
	var out SendResult
	if err := c.api.PostWithContext(ctx, svcPath(req.Service, "jobs"), body, &out); err != nil {
		return SendResult{}, classify(err)
	}
	return out, nil
}
