package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"smsh/internal/service"
)

var statusLabel = map[string]string{"delivered": "Délivré", "sent": "Envoyé", "scheduled": "Programmé"}

// filterFlags déclare les filtres communs à « ls » et « rm --match ».
func filterFlags(f *pflag.FlagSet, q *service.HistoryQuery) {
	f.StringVarP(&q.Query, "query", "q", "", "texte recherché (message, numéro, expéditeur)")
	f.StringVar(&q.Receiver, "to", "", "destinataire")
	f.StringVar(&q.Status, "status", "", "delivered | sent | scheduled")
	f.StringVar(&q.Sender, "sender", "", "expéditeur")
	f.StringVar(&q.Tag, "tag", "", "tag (ID de campagne Smsh)")
	f.StringVar(&q.DateFrom, "from", "", "à partir du (AAAA-MM-JJ)")
	f.StringVar(&q.DateTo, "until", "", "jusqu'au (AAAA-MM-JJ), inclus")
}

// syncHistory télécharge les nouveaux SMS jusqu'à épuisement, avec progression sur stderr.
func syncHistory(ctx context.Context, svc *service.Service, days int) (service.SyncResult, error) {
	var total service.SyncResult
	for {
		res, err := svc.SyncHistory(ctx, days)
		if err != nil {
			return total, err
		}
		total.Listed = res.Listed
		total.Fetched += res.Fetched
		total.Removed += res.Removed
		total.Remaining = res.Remaining
		total.Errors = append(total.Errors, res.Errors...)
		if !asJSON {
			fmt.Fprintf(os.Stderr, "\rSynchronisation : %d téléchargé(s), %d restant(s)…   ", total.Fetched, res.Remaining)
		}
		if res.Remaining == 0 || res.Fetched == 0 || ctx.Err() != nil {
			break
		}
	}
	if !asJSON {
		fmt.Fprintln(os.Stderr)
	}
	return total, ctx.Err()
}

func syncCmd() *cobra.Command {
	var days int
	c := &cobra.Command{
		Use:   "sync",
		Short: "Met à jour le cache local de l'historique depuis OVH",
		Args:  exactly(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withService(func(ctx context.Context, svc *service.Service) error {
				res, err := syncHistory(ctx, svc, days)
				if err != nil {
					return err
				}
				if asJSON {
					printJSON(os.Stdout, res)
				} else {
					fmt.Printf("%d SMS connus d'OVH sur %d jours, %d téléchargé(s), %d supprimé(s) du cache, %d restant(s).\n",
						res.Listed, days, res.Fetched, res.Removed, res.Remaining)
					for _, e := range res.Errors {
						fmt.Fprintln(os.Stderr, "  ! "+e)
					}
				}
				if len(res.Errors) > 0 {
					return failf("%d erreur(s) pendant la synchronisation", len(res.Errors))
				}
				return nil
			})
		},
	}
	c.Flags().IntVar(&days, "days", 30, "nombre de jours à synchroniser")
	return c
}

func lsCmd() *cobra.Command {
	var (
		q            service.HistoryQuery
		limit, off   int
		doSync, full bool
		days         int
	)
	c := &cobra.Command{
		Use:   "ls",
		Short: "Liste les SMS envoyés (cache local, du plus récent au plus ancien)",
		Example: `  smsh ls --limit 50
  smsh ls --sync --to 0612345678
  smsh ls --status sent --from 2026-11-01 --json`,
		Args: exactly(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withService(func(ctx context.Context, svc *service.Service) error {
				if doSync {
					if _, err := syncHistory(ctx, svc, days); err != nil {
						return err
					}
				}
				page, err := svc.SearchHistory(ctx, q, off, limit)
				if err != nil {
					return err
				}
				if asJSON {
					printJSON(os.Stdout, page)
					return nil
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tDATE\tDESTINATAIRE\tSTATUT\tCR.\tMESSAGE")
				for _, h := range page.Items {
					msg := trunc(h.Message, 60)
					if full {
						msg = trunc(h.Message, 0)
					}
					fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%v\t%s\n", h.ID, dateFr(h.CreatedAt), h.Receiver, statusLabel[h.Status], h.Credits, msg)
				}
				if err := tw.Flush(); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "%d–%d sur %d SMS · %v crédit(s) au total\n",
					min(off+1, page.Total), off+len(page.Items), page.Total, page.Credits)
				return nil
			})
		},
	}
	filterFlags(c.Flags(), &q)
	f := c.Flags()
	f.IntVarP(&limit, "limit", "n", 20, "nombre de lignes")
	f.IntVar(&off, "offset", 0, "ignorer les N premières lignes")
	f.BoolVar(&doSync, "sync", false, "synchroniser l'historique avant d'afficher")
	f.IntVar(&days, "days", 7, "jours à synchroniser avec --sync")
	f.BoolVar(&full, "full", false, "ne pas tronquer les messages")
	return c
}

func rmCmd() *cobra.Command {
	var (
		q     service.HistoryQuery
		match bool
		yes   bool
		dry   bool
	)
	c := &cobra.Command{
		Use:   "rm <id>... | --match [filtres]",
		Short: "Supprime des SMS de l'historique chez OVH et dans le cache local",
		Example: `  smsh rm 12345 12346
  smsh rm --match --status sent --until 2026-09-30 --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if match == (len(args) > 0) {
				return usagef("indiquez soit des identifiants, soit --match avec des filtres")
			}
			return withService(func(ctx context.Context, svc *service.Service) error {
				var ids []int64
				if match {
					if q == (service.HistoryQuery{}) {
						return usagef("--match exige au moins un filtre (--query, --to, --status, --sender, --tag, --from, --until)")
					}
					var err error
					if ids, err = svc.HistoryMatchingIDs(ctx, q); err != nil {
						return err
					}
				} else {
					for _, a := range args {
						n, err := strconv.ParseInt(a, 10, 64)
						if err != nil {
							return usagef("identifiant invalide : %q", a)
						}
						ids = append(ids, n)
					}
				}
				if len(ids) == 0 {
					if asJSON {
						printJSON(os.Stdout, service.DeleteResult{Failed: []service.DeleteFailure{}})
					} else {
						fmt.Println("Aucun SMS à supprimer.")
					}
					return nil
				}
				if dry {
					fmt.Fprintf(os.Stderr, "Simulation : %d SMS seraient supprimés.\n", len(ids))
					return nil
				}
				if err := confirm(fmt.Sprintf("Supprimer définitivement %d SMS chez OVH ?", len(ids)), yes); err != nil {
					return err
				}
				cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				defer cancel()
				res, err := svc.DeleteHistory(cctx, ids)
				if asJSON {
					printJSON(os.Stdout, res)
				} else {
					fmt.Printf("%d SMS supprimé(s).\n", res.Deleted)
					for _, f := range res.Failed {
						fmt.Fprintf(os.Stderr, "  ! %d : %s\n", f.ID, f.Error)
					}
				}
				if err != nil {
					return err
				}
				if len(res.Failed) > 0 {
					return failf("%d suppression(s) en échec", len(res.Failed))
				}
				return nil
			})
		},
	}
	filterFlags(c.Flags(), &q)
	c.Flags().BoolVar(&match, "match", false, "supprimer tous les SMS correspondant aux filtres")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "ne pas demander de confirmation")
	c.Flags().BoolVar(&dry, "dry-run", false, "compter sans supprimer")
	return c
}
