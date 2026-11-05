package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"smsh/internal/service"
	"smsh/internal/store"
)

var campStatusLabel = map[string]string{
	store.StatusPending: "en attente", store.StatusRunning: "en cours", store.StatusPaused: "en pause",
	store.StatusCompleted: "terminée", store.StatusCanceled: "annulée", store.StatusFailed: "échec",
}

func campaignCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "campaign",
		Short: "Campagnes SMS à partir d'un fichier CSV",
		Long: `Campagnes SMS à partir d'un fichier CSV.

Deux modes :
  batch  même message pour tous, traité côté OVH (continue si vous fermez le terminal) ;
  local  un SMS par destinataire (variables {colonne}), débit limité, exécuté par ce processus :
         Ctrl-C met la campagne en pause, « smsh campaign resume <id> » la reprend.`,
	}
	c.AddCommand(campRunCmd(), campLsCmd(), campShowCmd(), campResumeCmd(), campCancelCmd(), campRetryCmd(), campRmCmd())
	return c
}

// ---------- run ----------

func campRunCmd() *cobra.Command {
	var (
		message, file, name, phone, mode, sender, at string
		rate                                         float64
		stop, yes, dry, wait                         bool
	)
	c := &cobra.Command{
		Use:   "run <fichier.csv>",
		Short: "Simule puis lance une campagne",
		Example: `  smsh campaign run contacts.csv -m "Bonjour {prenom}, rendez-vous demain." --dry-run
  smsh campaign run contacts.csv -f message.txt --name "Relance" --phone-col telephone --yes
  smsh campaign run liste.csv -m "Promo -20%" --mode batch --at "2026-11-10 09:00" --stop --yes`,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usagef("indiquez le fichier CSV : smsh campaign run <fichier.csv> -m \"message\"")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			msg, err := readMessage(message, file)
			if err != nil {
				return err
			}
			if mode != "" && mode != store.ModeBatch && mode != store.ModeLocal {
				return usagef("--mode : %q ou %q", store.ModeBatch, store.ModeLocal)
			}
			return withService(func(ctx context.Context, svc *service.Service) error {
				info, err := svc.ImportCSV(args[0])
				if err != nil {
					return err
				}
				col, err := phoneColumn(info, phone)
				if err != nil {
					return err
				}
				plan := service.PlanInput{ImportID: info.ID, PhoneColumn: col, Template: msg, Mode: mode, StopClause: stop, Rate: rate}
				cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
				rep, err := svc.SimulateCampaign(cctx, plan)
				cancel()
				if err != nil {
					return err
				}
				if !asJSON {
					printReport(info, rep)
				}
				if len(rep.Blocking) > 0 {
					if asJSON {
						printJSON(os.Stdout, map[string]any{"dryRun": dry, "report": rep})
					}
					return failf("simulation bloquée : %s", strings.Join(rep.Blocking, " "))
				}
				if dry {
					if asJSON {
						printJSON(os.Stdout, map[string]any{"dryRun": true, "report": rep})
					} else {
						fmt.Fprintln(os.Stderr, "Simulation : rien n'a été envoyé.")
					}
					return nil
				}
				if name == "" {
					name = strings.TrimSuffix(info.File, fileExt(info.File))
				}
				if err := confirm(fmt.Sprintf("Lancer la campagne « %s » (%d SMS, mode %s) ?", name, rep.Recipients, rep.Mode), yes); err != nil {
					return err
				}
				lctx, lcancel := context.WithTimeout(ctx, 2*time.Minute)
				camp, err := svc.LaunchCampaign(lctx, service.LaunchInput{PlanInput: plan, Name: name, Sender: sender, SendAt: normalizeAt(at)})
				lcancel()
				if err != nil {
					return err
				}
				if camp.Mode == store.ModeBatch && !wait {
					if asJSON {
						printJSON(os.Stdout, camp)
					} else {
						fmt.Printf("Campagne « %s » créée (batch OVH %s). Suivi : smsh campaign show %s\n", camp.Name, camp.BatchID, camp.ID)
					}
					return nil
				}
				final, err := follow(ctx, svc, camp)
				if asJSON {
					printJSON(os.Stdout, final)
				}
				return finishErr(final, err)
			})
		},
	}
	f := c.Flags()
	f.StringVarP(&message, "message", "m", "", "modèle du message, variables {colonne}")
	f.StringVarP(&file, "file", "f", "", "lire le modèle dans un fichier (« - » : entrée standard)")
	f.StringVar(&name, "name", "", "nom de la campagne (défaut : nom du fichier)")
	f.StringVar(&phone, "phone-col", "", "colonne des numéros : nom d'en-tête ou position (1 = première) ; détectée par défaut")
	f.StringVar(&mode, "mode", "", "batch | local (défaut : batch, ou local si le modèle a des variables)")
	f.Float64Var(&rate, "rate", 3, "débit en SMS/seconde (mode local)")
	f.StringVar(&sender, "sender", "", "expéditeur (vide : numéro court par défaut)")
	f.StringVar(&at, "at", "", "envoi différé : « 2026-11-10 09:00 » (heure locale) ou RFC 3339")
	f.BoolVar(&stop, "stop", false, "ajouter la mention STOP (SMS commercial)")
	f.BoolVarP(&yes, "yes", "y", false, "ne pas demander de confirmation")
	f.BoolVar(&dry, "dry-run", false, "simuler seulement (rien n'est envoyé)")
	f.BoolVar(&wait, "wait", false, "mode batch : attendre la fin de l'envoi")
	return c
}

func fileExt(name string) string {
	if i := strings.LastIndex(name, "."); i > 0 {
		return name[i:]
	}
	return ""
}

func phoneColumn(info service.ImportInfo, spec string) (int, error) {
	if spec == "" {
		if info.PhoneColumn < 0 {
			return 0, usagef("colonne des numéros non détectée : précisez --phone-col (en-têtes : %s)", strings.Join(info.Headers, ", "))
		}
		return info.PhoneColumn, nil
	}
	for i, h := range info.Headers {
		if strings.EqualFold(strings.TrimSpace(h), spec) {
			return i, nil
		}
	}
	if n, err := strconv.Atoi(spec); err == nil && n >= 1 && n <= len(info.Headers) {
		return n - 1, nil
	}
	return 0, usagef("colonne introuvable : %q (en-têtes : %s)", spec, strings.Join(info.Headers, ", "))
}

func printReport(info service.ImportInfo, r service.PlanReport) {
	w := os.Stderr
	fmt.Fprintf(w, "Fichier       : %s (%d ligne(s))\n", info.File, info.Rows)
	fmt.Fprintf(w, "Destinataires : %d valide(s)", r.Recipients)
	if r.InvalidCount > 0 {
		fmt.Fprintf(w, ", %d invalide(s)", r.InvalidCount)
	}
	if r.Dupes > 0 {
		fmt.Fprintf(w, ", %d doublon(s) ignoré(s)", r.Dupes)
	}
	fmt.Fprintf(w, "\nMode          : %s", r.Mode)
	if len(r.Variables) > 0 {
		fmt.Fprintf(w, " (variables : %s)", strings.Join(r.Variables, ", "))
	}
	fmt.Fprintf(w, "\nSegments      : %d au total (max %d par SMS)\n", r.TotalParts, r.MaxParts)
	if r.TotalCredits > 0 {
		fmt.Fprintf(w, "Coût estimé   : %v crédit(s) ≈ %s HT\n", r.TotalCredits, euro(r.PriceMin, r.PriceMax, r.Currency))
	}
	if r.DurationSeconds >= 1 {
		fmt.Fprintf(w, "Durée estimée : %s\n", time.Duration(r.DurationSeconds*float64(time.Second)).Round(time.Second))
	}
	for _, s := range r.Samples[:min(len(r.Samples), 2)] {
		fmt.Fprintf(w, "Aperçu        : %s → %s\n", s.Phone, trunc(s.Message, 80))
	}
	for _, i := range r.Invalid[:min(len(r.Invalid), 5)] {
		fmt.Fprintf(w, "  ! ligne %d : « %s » ignorée\n", i.Line, i.Value)
	}
	for _, x := range r.Warnings {
		fmt.Fprintln(w, "  ! "+x)
	}
	for _, x := range r.Blocking {
		fmt.Fprintln(w, "  ✗ "+x)
	}
}

// follow suit une campagne jusqu'à sa fin. Mode local : Ctrl-C met en pause proprement.
func follow(ctx context.Context, svc *service.Service, c store.Campaign) (store.Campaign, error) {
	local := c.Mode == store.ModeLocal
	done := make(chan struct{})
	if local {
		go func() { svc.Runner.Wait(c.ID); close(done) }()
	}
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	tty := interactive()
	last := time.Time{}
	show := func(c store.Campaign) {
		if asJSON {
			return
		}
		line := fmt.Sprintf("%s : %d/%d envoyés, %d échec(s) — %s", c.Name, c.Sent, c.Total, c.Failed, campStatusLabel[c.Status])
		if tty {
			fmt.Fprintf(os.Stderr, "\r%-100s", line)
		} else if time.Since(last) > 5*time.Second {
			fmt.Fprintln(os.Stderr, line)
			last = time.Now()
		}
	}
	fetch := func() store.Campaign {
		if cur, err := svc.GetCampaign(context.Background(), c.ID); err == nil {
			return cur
		}
		return c
	}
	interrupted := false
	for {
		select {
		case <-ctx.Done():
			if !interrupted {
				interrupted = true
				if local {
					_ = svc.Runner.Pause(c.ID)
				}
			}
			if !local {
				return fetch(), ctx.Err()
			}
			ctx = context.Background() // attendre la fin effective de la pause
		case <-done:
			final := fetch()
			show(final)
			if tty && !asJSON {
				fmt.Fprintln(os.Stderr)
			}
			if interrupted {
				return final, failf("interrompu : campagne en pause, reprenez avec « smsh campaign resume %s »", c.ID)
			}
			return final, nil
		case <-tick.C:
			cur := fetch()
			show(cur)
			if !local && !store.Active(cur.Status) {
				if tty && !asJSON {
					fmt.Fprintln(os.Stderr)
				}
				return cur, nil
			}
		}
	}
}

// finishErr traduit l'état final d'une campagne en code de sortie.
func finishErr(c store.Campaign, err error) error {
	if err != nil {
		return err
	}
	if !asJSON {
		fmt.Printf("Campagne « %s » : %s — %d/%d envoyés, %d échec(s), %v crédit(s).\n",
			c.Name, campStatusLabel[c.Status], c.Sent, c.Total, c.Failed, c.Credits)
		if c.Error != "" {
			fmt.Fprintln(os.Stderr, "  ! "+c.Error)
		}
	}
	switch {
	case c.Status == store.StatusPaused:
		return failf("campagne en pause : %s (reprise : smsh campaign resume %s)", orDefault(c.Error, "voir les détails"), c.ID)
	case c.Status == store.StatusFailed:
		return failf("campagne en échec : %s", c.Error)
	case c.Failed > 0:
		return failf("%d destinataire(s) en échec (relance : smsh campaign retry %s)", c.Failed, c.ID)
	}
	return nil
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// ---------- ls / show ----------

func campLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "Liste les campagnes",
		Args:  exactly(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withService(func(ctx context.Context, svc *service.Service) error {
				list, err := svc.ListCampaigns(ctx)
				if err != nil {
					return err
				}
				if asJSON {
					printJSON(os.Stdout, list)
					return nil
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tNOM\tMODE\tSTATUT\tENVOYÉS\tÉCHECS\tCRÉDITS\tCRÉÉE")
				for _, c := range list {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d/%d\t%d\t%v\t%s\n", c.ID, trunc(c.Name, 30), c.Mode,
						campStatusLabel[c.Status], c.Sent, c.Total, c.Failed, c.Credits, dateFr(c.CreatedAt))
				}
				return tw.Flush()
			})
		},
	}
}

func campShowCmd() *cobra.Command {
	var recipients bool
	var status string
	var limit int
	c := &cobra.Command{
		Use:   "show <id>",
		Short: "Détail d'une campagne (--recipients : liste des destinataires)",
		Args:  exactly(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(ctx context.Context, svc *service.Service) error {
				camp, err := svc.GetCampaign(ctx, args[0])
				if err != nil {
					return err
				}
				var page service.RecipientsPage
				if recipients {
					if page, err = svc.CampaignRecipients(ctx, args[0], status, 0, limit); err != nil {
						return err
					}
				}
				if asJSON {
					printJSON(os.Stdout, map[string]any{"campaign": camp, "recipients": page})
					return nil
				}
				fmt.Printf("Nom      : %s\nID       : %s\nMode     : %s\nStatut   : %s\n", camp.Name, camp.ID, camp.Mode, campStatusLabel[camp.Status])
				fmt.Printf("Envoyés  : %d/%d (%d échec(s))\nCrédits  : %v\nCréée    : %s\n", camp.Sent, camp.Total, camp.Failed, camp.Credits, dateFr(camp.CreatedAt))
				if camp.SendAt != nil {
					fmt.Printf("Envoi    : %s\n", dateFr(*camp.SendAt))
				}
				fmt.Printf("Message  : %s\n", trunc(camp.Template, 120))
				if camp.Error != "" {
					fmt.Printf("Erreur   : %s\n", camp.Error)
				}
				if recipients {
					tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
					fmt.Fprintln(tw, "\nN°\tNUMÉRO\tSTATUT\tDÉTAIL")
					for _, r := range page.Items {
						fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", r.Idx+1, r.Phone, r.Status, trunc(r.Error, 80))
					}
					_ = tw.Flush()
					fmt.Fprintf(os.Stderr, "%d affiché(s) sur %d\n", len(page.Items), page.Total)
				}
				return nil
			})
		},
	}
	c.Flags().BoolVar(&recipients, "recipients", false, "lister les destinataires (campagnes en mode local)")
	c.Flags().StringVar(&status, "status", "", "filtrer : pending | sent | failed")
	c.Flags().IntVarP(&limit, "limit", "n", 50, "nombre de destinataires")
	return c
}

// ---------- reprise / annulation / relance / suppression ----------

func campResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <id>",
		Short: "Reprend une campagne locale en pause (exécutée dans ce terminal)",
		Args:  exactly(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(ctx context.Context, svc *service.Service) error {
				camp, err := svc.GetCampaign(ctx, args[0])
				if err != nil {
					return err
				}
				if camp.Mode != store.ModeLocal {
					return failf("seules les campagnes en mode local peuvent être reprises")
				}
				if err := svc.ResumeCampaign(ctx, camp.ID); err != nil {
					return err
				}
				final, err := follow(ctx, svc, camp)
				if asJSON {
					printJSON(os.Stdout, final)
				}
				return finishErr(final, err)
			})
		},
	}
}

func campRetryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "retry <id>",
		Short: "Relance les destinataires en échec d'une campagne locale",
		Args:  exactly(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(ctx context.Context, svc *service.Service) error {
				n, err := svc.RetryFailed(ctx, args[0])
				if err != nil {
					return err
				}
				if n == 0 {
					fmt.Println("Aucun destinataire en échec.")
					return nil
				}
				camp, err := svc.GetCampaign(ctx, args[0])
				if err != nil {
					return err
				}
				final, err := follow(ctx, svc, camp)
				if asJSON {
					printJSON(os.Stdout, final)
				}
				return finishErr(final, err)
			})
		},
	}
}

func campCancelCmd() *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:   "cancel <id>",
		Short: "Annule une campagne (batch différé : annulation côté OVH)",
		Args:  exactly(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := confirm("Annuler cette campagne ?", yes); err != nil {
				return err
			}
			return withService(func(ctx context.Context, svc *service.Service) error {
				if err := svc.CancelCampaign(ctx, args[0]); err != nil {
					return err
				}
				if asJSON {
					printJSON(os.Stdout, map[string]string{"canceled": args[0]})
				} else {
					fmt.Println("Campagne annulée.")
				}
				return nil
			})
		},
	}
	c.Flags().BoolVarP(&yes, "yes", "y", false, "ne pas demander de confirmation")
	return c
}

func campRmCmd() *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:   "rm <id>",
		Short: "Supprime une campagne de l'historique local (rien n'est supprimé chez OVH)",
		Args:  exactly(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := confirm("Supprimer cette campagne de l'historique local ?", yes); err != nil {
				return err
			}
			return withService(func(ctx context.Context, svc *service.Service) error {
				if err := svc.DeleteCampaign(ctx, args[0]); err != nil {
					return err
				}
				if asJSON {
					printJSON(os.Stdout, map[string]string{"deleted": args[0]})
				} else {
					fmt.Println("Campagne supprimée.")
				}
				return nil
			})
		},
	}
	c.Flags().BoolVarP(&yes, "yes", "y", false, "ne pas demander de confirmation")
	return c
}
