package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"smsh/internal/service"
)

func sendCmd() *cobra.Command {
	var (
		message, file, sender, at string
		stop, yes, dry            bool
	)
	c := &cobra.Command{
		Use:   "send <numéro>... [flags]",
		Short: "Envoie un SMS à 5 numéros au maximum (utilisez « campaign run » pour plus)",
		Example: `  smsh send 0612345678 -m "Bonjour"
  smsh send +33612345678 +14155552671 -f message.txt --yes
  echo "Rappel" | smsh send 0612345678 -f - --at "2026-11-06 09:00" --yes`,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return usagef("indiquez au moins un numéro")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			msg, err := readMessage(message, file)
			if err != nil {
				return err
			}
			in := service.SendInput{
				Message: msg, Recipients: strings.Join(args, "\n"),
				Sender: sender, SendAt: normalizeAt(at), StopClause: stop,
			}
			return withService(func(ctx context.Context, svc *service.Service) error {
				cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
				est, estErr := svc.EstimateCost(cctx, service.CostInput{Message: in.Message, Recipients: in.Recipients, StopClause: in.StopClause})
				if !asJSON {
					fmt.Fprintf(os.Stderr, "Message      : %s\n", trunc(msg, 100))
					fmt.Fprintf(os.Stderr, "Destinataires: %s\n", strings.Join(args, ", "))
					if estErr == nil && est.Recipients > 0 {
						fmt.Fprintf(os.Stderr, "Segments     : %d (%s, %d caractères)\n", est.Parts, est.Encoding, est.Characters)
						fmt.Fprintf(os.Stderr, "Coût estimé  : %v crédit(s) ≈ %s HT\n", est.TotalCredits, euro(est.PriceMin, est.PriceMax, est.Currency))
					} else if estErr != nil {
						fmt.Fprintf(os.Stderr, "Coût         : non estimé (%s)\n", service.Describe(estErr).Message)
					}
					if in.SendAt != "" {
						fmt.Fprintf(os.Stderr, "Envoi différé: %s\n", in.SendAt)
					}
				}
				if dry {
					if asJSON {
						printJSON(os.Stdout, map[string]any{"dryRun": true, "estimate": est})
					} else {
						fmt.Fprintln(os.Stderr, "Simulation : rien n'a été envoyé.")
					}
					return nil
				}
				if err := confirm("Envoyer ?", yes); err != nil {
					return err
				}
				out, err := svc.Send(cctx, in)
				if err != nil {
					return err
				}
				if asJSON {
					printJSON(os.Stdout, out)
					return nil
				}
				verb := "envoyé(s)"
				if out.Scheduled {
					verb = "programmé(s)"
				}
				fmt.Printf("%d SMS %s à %s — %v crédit(s) débité(s)\n", len(out.Sent), verb, strings.Join(out.Sent, ", "), out.CreditsRemoved)
				if len(out.Rejected) > 0 {
					fmt.Printf("Refusé(s) par OVH : %s\n", strings.Join(out.Rejected, ", "))
					return failf("%d destinataire(s) refusé(s)", len(out.Rejected))
				}
				return nil
			})
		},
	}
	f := c.Flags()
	f.StringVarP(&message, "message", "m", "", "texte du SMS")
	f.StringVarP(&file, "file", "f", "", "lire le message dans un fichier (« - » : entrée standard)")
	f.StringVar(&sender, "sender", "", "expéditeur (vide : numéro court par défaut)")
	f.StringVar(&at, "at", "", "envoi différé : « 2026-11-06 09:00 » (heure locale) ou RFC 3339")
	f.BoolVar(&stop, "stop", false, "ajouter la mention STOP (SMS commercial)")
	f.BoolVarP(&yes, "yes", "y", false, "ne pas demander de confirmation")
	f.BoolVar(&dry, "dry-run", false, "afficher le coût sans envoyer")
	return c
}
