package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"smsh/internal/creds"
	"smsh/internal/ovh"
)

func newTestService(t *testing.T) (*Service, *ovh.Mock) {
	t.Helper()
	mock := ovh.NewMock()
	m := creds.NewManager(t.TempDir(), creds.NewMemoryVault())
	_, err := m.Save(creds.Input{Name: "T", ServiceName: "sms-demo-1", Endpoint: "ovh-eu",
		AppKey: "AK-1234567890", AppSecret: "AS-1234567890", ConsumerKey: "CK-1234567890"})
	if err != nil {
		t.Fatal(err)
	}
	return &Service{
		Creds:     m,
		NewClient: func(ovh.Credentials) (ovh.Client, error) { return mock, nil },
		Now:       time.Now,
	}, mock
}

func TestSend(t *testing.T) {
	s, mock := newTestService(t)
	out, err := s.Send(context.Background(), SendInput{
		Message: "Bonjour", Recipients: "06 12 34 56 78\n0612345678, 0698765432", Sender: "Smsh",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Sent) != 2 || out.CreditsRemoved != 2 || out.Scheduled || out.Analysis.Segments != 1 {
		t.Fatalf("out = %+v", out)
	}
	if got := mock.Sent[0].Receivers; got[0] != "+33612345678" || len(got) != 2 {
		t.Errorf("receivers = %v", got)
	}
}

func TestSendUnicodeAndScheduled(t *testing.T) {
	s, mock := newTestService(t)
	at := time.Now().Add(time.Hour).Format("2006-01-02T15:04")
	out, err := s.Send(context.Background(), SendInput{Message: "Salut 😀", Recipients: "0612345678", SendAt: at})
	if err != nil || !out.Scheduled {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if !mock.Sent[0].Unicode || mock.Sent[0].SendAt == nil {
		t.Errorf("req = %+v", mock.Sent[0])
	}
}

func TestSendValidation(t *testing.T) {
	s, _ := newTestService(t)
	for name, in := range map[string]SendInput{
		"message vide": {Message: " ", Recipients: "0612345678"},
		"sans dest":    {Message: "x"},
		"numéro faux":  {Message: "x", Recipients: "0612345678, 123"},
		"date fausse":  {Message: "x", Recipients: "0612345678", SendAt: "demain"},
	} {
		if _, err := s.Send(context.Background(), in); err == nil {
			t.Errorf("%s : erreur attendue", name)
		}
	}
}

func TestSendErrorsAreDescribed(t *testing.T) {
	s, mock := newTestService(t)
	mock.Info.CreditsLeft = 0
	_, err := s.Send(context.Background(), SendInput{Message: "x", Recipients: "0612345678"})
	if p := Describe(err); p.Kind != "insufficient_credits" {
		t.Fatalf("problem = %+v", p)
	}
	mock.Err = &ovh.Error{Kind: ovh.KindCredentials, Message: "refusé"}
	_, err = s.Overview(context.Background())
	if p := Describe(err); p.Kind != "credentials" {
		t.Fatalf("problem = %+v", p)
	}
}

func TestTestConnectionDoesNotPersist(t *testing.T) {
	s, _ := newTestService(t)
	info, err := s.TestConnection(context.Background(), creds.Input{
		Name: "Autre", ServiceName: "sms-x", Endpoint: "ovh-eu", AppKey: "a", AppSecret: "b", ConsumerKey: "c",
	})
	if err != nil || info.CreditsLeft != 100 || len(info.Senders) != 2 {
		t.Fatalf("info=%+v err=%v", info, err)
	}
	if st, _ := s.Creds.Status(); st.Accounts != 1 {
		t.Errorf("accounts = %d", st.Accounts)
	}
}

func TestDescribeCredsErrors(t *testing.T) {
	cases := map[error]string{
		creds.ErrLocked:        "locked",
		creds.ErrNoAccount:     "no_account",
		creds.ErrBadPassphrase: "bad_passphrase",
		errors.New("boom"):     "internal",
	}
	for err, want := range cases {
		if got := Describe(err).Kind; got != want {
			t.Errorf("%v -> %s, want %s", err, got, want)
		}
	}
}
