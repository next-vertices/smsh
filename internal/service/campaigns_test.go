package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"smsh/internal/store"
)

func writeCSV(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "contacts.csv")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const sampleCSV = "prenom;tel;ville\nJean;0612345678;Lyon\nLéa;06 98 76 54 32;Paris\nBob;abc;Nice\nJean bis;+33612345678;Lyon\nSam;+14155552671;NYC\n"

func importSample(t *testing.T, s *Service) ImportInfo {
	t.Helper()
	info, err := s.ImportCSV(writeCSV(t, sampleCSV))
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func TestImportCSV(t *testing.T) {
	s, _ := newTestService(t)
	info := importSample(t, s)
	if info.Rows != 5 || info.PhoneColumn != 1 || len(info.Headers) != 3 || info.File != "contacts.csv" || len(info.Sample) != 5 {
		t.Fatalf("info = %+v", info)
	}
	if _, err := s.ImportCSV(filepath.Join(t.TempDir(), "absent.csv")); err == nil {
		t.Error("fichier absent : erreur attendue")
	}
}

func TestSimulateLocalWithVariables(t *testing.T) {
	s, _ := newTestService(t)
	info := importSample(t, s)
	rep, err := s.SimulateCampaign(context.Background(), PlanInput{
		ImportID: info.ID, PhoneColumn: 1, Template: "Bonjour {prenom} de {ville} !", Rate: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Mode != store.ModeLocal || rep.Recipients != 3 || rep.InvalidCount != 1 || rep.Dupes != 1 {
		t.Fatalf("rep = %+v", rep)
	}
	if len(rep.Samples) != 3 || rep.Samples[0].Message != "Bonjour Jean de Lyon !" || rep.Samples[0].Phone != "+33612345678" {
		t.Fatalf("samples = %+v", rep.Samples)
	}
	// 2 FR (1 crédit) + 1 US (0,35), 1 segment chacun.
	if rep.TotalCredits != 2.35 || rep.TotalParts != 3 || len(rep.Lines) != 2 || rep.PriceMax <= 0 {
		t.Fatalf("coût : %+v", rep)
	}
	if rep.DurationSeconds != 1.5 || len(rep.Blocking) != 0 {
		t.Fatalf("durée/blocage : %+v", rep)
	}
}

func TestSimulateBlockingProblems(t *testing.T) {
	s, mock := newTestService(t)
	info := importSample(t, s)
	ctx := context.Background()
	cases := map[string]PlanInput{
		"variable inconnue": {ImportID: info.ID, PhoneColumn: 1, Template: "Hello {nom}"},
		"batch + variables": {ImportID: info.ID, PhoneColumn: 1, Template: "Hello {prenom}", Mode: store.ModeBatch},
		"message vide":      {ImportID: info.ID, PhoneColumn: 1, Template: " "},
		"import inconnu":    {ImportID: "zzz", PhoneColumn: 1, Template: "x"},
	}
	for name, in := range cases {
		rep, err := s.SimulateCampaign(ctx, in)
		if name == "import inconnu" {
			if err == nil {
				t.Errorf("%s : erreur attendue", name)
			}
			continue
		}
		if err != nil || len(rep.Blocking) == 0 {
			t.Errorf("%s : blocage attendu, rep=%+v err=%v", name, rep, err)
		}
	}
	mock.Info.CreditsLeft = 1
	rep, _ := s.SimulateCampaign(ctx, PlanInput{ImportID: info.ID, PhoneColumn: 1, Template: "Salut"})
	if len(rep.Blocking) != 1 || !strings.Contains(rep.Blocking[0], "Crédits insuffisants") {
		t.Fatalf("crédits : %+v", rep.Blocking)
	}
}

func TestLaunchLocalCampaignEndToEnd(t *testing.T) {
	s, mock := newTestService(t)
	info := importSample(t, s)
	ctx := context.Background()
	c, err := s.LaunchCampaign(ctx, LaunchInput{
		PlanInput: PlanInput{ImportID: info.ID, PhoneColumn: 1, Template: "Salut {prenom}", Rate: 1000},
		Name:      "Test", Sender: "Smsh",
	})
	if err != nil {
		t.Fatal(err)
	}
	s.Runner.Wait(c.ID)
	got, _ := s.GetCampaign(ctx, c.ID)
	if got.Mode != store.ModeLocal || got.Status != store.StatusCompleted || got.Sent != 3 || got.Failed != 0 {
		t.Fatalf("campagne = %+v", got)
	}
	if got.Credits != 3 { // le mock facture 1 crédit par destinataire
		t.Errorf("crédits = %v", got.Credits)
	}
	if len(mock.Sent) != 3 || mock.Sent[1].Message != "Salut Léa" || mock.Sent[1].Sender != "Smsh" || len(mock.Sent[1].Receivers) != 1 {
		t.Fatalf("envois = %+v", mock.Sent)
	}
	page, err := s.CampaignRecipients(ctx, c.ID, store.RecipientSent, 0, 10)
	if err != nil || page.Total != 3 || page.Items[0].OvhID == 0 {
		t.Fatalf("page = %+v err=%v", page, err)
	}
	if err := s.DeleteCampaign(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetCampaign(ctx, c.ID); err == nil {
		t.Error("campagne supprimée : erreur attendue")
	}
}

func TestLaunchBatchCampaign(t *testing.T) {
	s, mock := newTestService(t)
	info := importSample(t, s)
	ctx := context.Background()
	c, err := s.LaunchCampaign(ctx, LaunchInput{
		PlanInput: PlanInput{ImportID: info.ID, PhoneColumn: 1, Template: "Promo -20% ce week-end"},
		Name:      "Promo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != store.ModeBatch || c.BatchID == "" || c.Status != store.StatusRunning || c.Total != 3 || len(mock.Batches) != 1 {
		t.Fatalf("c = %+v", c)
	}
	got, err := s.GetCampaign(ctx, c.ID) // resynchronisation avec OVH
	if err != nil || got.Status != store.StatusCompleted || got.Sent != 3 || got.Total != 3 {
		t.Fatalf("got = %+v err=%v", got, err)
	}
	if err := s.PauseCampaign(ctx, c.ID); err == nil {
		t.Error("pause d'un batch : erreur attendue")
	}
}

func TestLaunchValidation(t *testing.T) {
	s, _ := newTestService(t)
	info := importSample(t, s)
	ctx := context.Background()
	base := PlanInput{ImportID: info.ID, PhoneColumn: 1, Template: "x"}
	if _, err := s.LaunchCampaign(ctx, LaunchInput{PlanInput: base}); err == nil {
		t.Error("nom manquant : erreur attendue")
	}
	bad := base
	bad.Template = "{inconnue}"
	if _, err := s.LaunchCampaign(ctx, LaunchInput{PlanInput: bad, Name: "n"}); err == nil {
		t.Error("variable inconnue : erreur attendue")
	}
	if cs, _ := s.ListCampaigns(ctx); len(cs) != 0 {
		t.Errorf("aucune campagne ne doit être créée en cas d'erreur : %+v", cs)
	}
}

func TestRetryFailedAndCancelLocal(t *testing.T) {
	s, mock := newTestService(t)
	info := importSample(t, s)
	ctx := context.Background()
	mock.Info.CreditsLeft = 2 // la 3e destination échouera : crédits insuffisants -> pause
	c, err := s.LaunchCampaign(ctx, LaunchInput{
		PlanInput: PlanInput{ImportID: info.ID, PhoneColumn: 1, Template: "Salut", Rate: 1000, Mode: store.ModeLocal},
		Name:      "Court",
	})
	// 3 destinataires pour 2,35 crédits : refusé dès la simulation (crédits insuffisants).
	if err == nil {
		s.Runner.Wait(c.ID)
		t.Fatalf("la simulation aurait dû bloquer : %+v", c)
	}
	mock.Info.CreditsLeft = 100
	c, err = s.LaunchCampaign(ctx, LaunchInput{
		PlanInput: PlanInput{ImportID: info.ID, PhoneColumn: 1, Template: "Salut", Rate: 1000, Mode: store.ModeLocal},
		Name:      "Ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	s.Runner.Wait(c.ID)
	if err := s.CancelCampaign(ctx, c.ID); err == nil {
		t.Error("annuler une campagne terminée : erreur attendue")
	}
	if n, err := s.RetryFailed(ctx, c.ID); err != nil || n != 0 {
		t.Errorf("rien à relancer : n=%d err=%v", n, err)
	}
}
