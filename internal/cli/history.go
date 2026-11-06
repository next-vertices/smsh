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

	"smsh/internal/i18n"
	"smsh/internal/service"
)

func statusLabel(s string) string {
	switch s {
	case "delivered", "sent", "scheduled":
		return i18n.T("cli.st_" + s)
	}
	return s
}

// filterFlags déclare les filtres communs à « ls » et « rm --match ».
func filterFlags(f *pflag.FlagSet, q *service.HistoryQuery) {
	f.StringVarP(&q.Query, "query", "q", "", i18n.T("cli.fl_query"))
	f.StringVar(&q.Receiver, "to", "", i18n.T("cli.fl_to"))
	f.StringVar(&q.Status, "status", "", "delivered | sent | scheduled")
	f.StringVar(&q.Sender, "sender", "", i18n.T("cli.fl_sender_f"))
	f.StringVar(&q.Tag, "tag", "", i18n.T("cli.fl_tag"))
	f.StringVar(&q.DateFrom, "from", "", i18n.T("cli.fl_from"))
	f.StringVar(&q.DateTo, "until", "", i18n.T("cli.fl_until"))
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
			fmt.Fprint(os.Stderr, i18n.T("cli.sync_progress", i18n.P{"fetched": total.Fetched, "remaining": res.Remaining}))
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
		Short: i18n.T("cli.sync_short"),
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
					fmt.Println(i18n.T("cli.sync_done", i18n.P{"listed": res.Listed, "days": days, "fetched": res.Fetched, "removed": res.Removed, "remaining": res.Remaining}))
					for _, e := range res.Errors {
						fmt.Fprintln(os.Stderr, "  ! "+e)
					}
				}
				if len(res.Errors) > 0 {
					return failf("%s", i18n.T("cli.sync_errors", i18n.P{"count": len(res.Errors)}))
				}
				return nil
			})
		},
	}
	c.Flags().IntVar(&days, "days", 30, i18n.T("cli.fl_days_sync"))
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
		Short: i18n.T("cli.ls_short"),
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
				fmt.Fprintln(tw, i18n.T("cli.th_ls"))
				for _, h := range page.Items {
					msg := trunc(h.Message, 60)
					if full {
						msg = trunc(h.Message, 0)
					}
					fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%v\t%s\n", h.ID, dateFr(h.CreatedAt), h.Receiver, statusLabel(h.Status), h.Credits, msg)
				}
				if err := tw.Flush(); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, i18n.T("cli.ls_footer", i18n.P{"from": min(off+1, page.Total), "to": off + len(page.Items), "total": page.Total, "credits": page.Credits}))
				return nil
			})
		},
	}
	filterFlags(c.Flags(), &q)
	f := c.Flags()
	f.IntVarP(&limit, "limit", "n", 20, i18n.T("cli.fl_limit"))
	f.IntVar(&off, "offset", 0, i18n.T("cli.fl_offset"))
	f.BoolVar(&doSync, "sync", false, i18n.T("cli.fl_sync"))
	f.IntVar(&days, "days", 7, i18n.T("cli.fl_days_ls"))
	f.BoolVar(&full, "full", false, i18n.T("cli.fl_full"))
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
		Use:   i18n.T("cli.rm_usage"),
		Short: i18n.T("cli.rm_short"),
		Example: `  smsh rm 12345 12346
  smsh rm --match --status sent --until 2026-09-30 --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if match == (len(args) > 0) {
				return usagef("%s", i18n.T("cli.rm_need"))
			}
			return withService(func(ctx context.Context, svc *service.Service) error {
				var ids []int64
				if match {
					if q == (service.HistoryQuery{}) {
						return usagef("%s", i18n.T("cli.rm_need_filter"))
					}
					var err error
					if ids, err = svc.HistoryMatchingIDs(ctx, q); err != nil {
						return err
					}
				} else {
					for _, a := range args {
						n, err := strconv.ParseInt(a, 10, 64)
						if err != nil {
							return usagef("%s", i18n.T("cli.rm_bad_id", i18n.P{"id": fmt.Sprintf("%q", a)}))
						}
						ids = append(ids, n)
					}
				}
				if len(ids) == 0 {
					if asJSON {
						printJSON(os.Stdout, service.DeleteResult{Failed: []service.DeleteFailure{}})
					} else {
						fmt.Println(i18n.T("cli.rm_none"))
					}
					return nil
				}
				if dry {
					fmt.Fprintln(os.Stderr, i18n.T("cli.rm_dry", i18n.P{"count": len(ids)}))
					return nil
				}
				if err := confirm(i18n.T("cli.rm_confirm", i18n.P{"count": len(ids)}), yes); err != nil {
					return err
				}
				cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				defer cancel()
				res, err := svc.DeleteHistory(cctx, ids)
				if asJSON {
					printJSON(os.Stdout, res)
				} else {
					fmt.Println(i18n.T("cli.rm_done", i18n.P{"count": res.Deleted}))
					for _, f := range res.Failed {
						fmt.Fprintf(os.Stderr, "  ! %d : %s\n", f.ID, f.Error)
					}
				}
				if err != nil {
					return err
				}
				if len(res.Failed) > 0 {
					return failf("%s", i18n.T("cli.rm_failed", i18n.P{"count": len(res.Failed)}))
				}
				return nil
			})
		},
	}
	filterFlags(c.Flags(), &q)
	c.Flags().BoolVar(&match, "match", false, i18n.T("cli.fl_match"))
	c.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("cli.fl_yes"))
	c.Flags().BoolVar(&dry, "dry-run", false, i18n.T("cli.fl_dry_count"))
	return c
}
