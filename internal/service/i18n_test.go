package service

import (
	"context"
	"strings"
	"testing"

	"smsh/internal/creds"
	"smsh/internal/i18n"
)

// Les messages du cœur suivent la langue courante ; le français reste la langue par défaut.
func TestMessagesFollowLanguage(t *testing.T) {
	defer i18n.SetLang(i18n.FR)

	i18n.SetLang("fr-FR")
	if got := Describe(creds.ErrLocked).Message; !strings.Contains(got, "verrouillé") {
		t.Errorf("fr = %q", got)
	}
	i18n.SetLang("en-GB")
	if got := Describe(creds.ErrLocked).Message; !strings.Contains(got, "locked") {
		t.Errorf("en = %q", got)
	}

	s, _ := newTestService(t)
	_, err := s.Send(context.Background(), SendInput{Message: "", Recipients: "0612345678"})
	if got := Describe(err).Message; got != "The message is empty." {
		t.Errorf("Send (en) = %q", got)
	}
	i18n.SetLang("fr-FR")
	_, err = s.Send(context.Background(), SendInput{Message: "", Recipients: "0612345678"})
	if got := Describe(err).Message; got != "Le message est vide." {
		t.Errorf("Send (fr) = %q", got)
	}
}
