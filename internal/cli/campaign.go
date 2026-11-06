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

	"smsh/internal/i18n"
	"smsh/internal/service"
	"smsh/internal/store"
)

func campStatusLabel(s string) string {
	switch s {
	case store.StatusPending, store.StatusRunning, store.StatusPaused, store.StatusCompleted, store.StatusCanceled, store.StatusFailed:
		return i18n.T("cli.cs_" + s)
	}
	return s
}

func campaignCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "campaign",
		Short: i18n.T("cli.camp_short"),
		Long:  i18n.T("cli.camp_long"),
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
		Use:     i18n.T("cli.run_usage"),
		Short:   i18n.T("cli.run_short"),
		Example: i18n.T("cli.run_example"),
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usagef("%s", i18n.T("cli.run_need_file"))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			msg, err := readMessage(message, file)
			if err != nil {
				return err
			}
			if mode != "" && mode != store.ModeBatch && mode != store.ModeLocal {
				return usagef("%s", i18n.T("cli.bad_mode", i18n.P{"a": fmt.Sprintf("%q", store.ModeBatch), "b": fmt.Sprintf("%q", store.ModeLocal)}))
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
					return failf("%s", i18n.T("cli.sim_blocked", i18n.P{"reasons": strings.Join(rep.Blocking, " ")}))
				}
				if dry {
					if asJSON {
						printJSON(os.Stdout, map[string]any{"dryRun": true, "report": rep})
					} else {
						fmt.Fprintln(os.Stderr, i18n.T("cli.dry_done"))
					}
					return nil
				}
				if name == "" {
					name = strings.TrimSuffix(info.File, fileExt(info.File))
				}
				if err := confirm(i18n.T("cli.run_confirm", i18n.P{"name": name, "count": rep.Recipients, "mode": rep.Mode}), yes); err != nil {
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
						fmt.Println(i18n.T("cli.run_created", i18n.P{"name": camp.Name, "batch": camp.BatchID, "id": camp.ID}))
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
	f.StringVarP(&message, "message", "m", "", i18n.T("cli.fl_template"))
	f.StringVarP(&file, "file", "f", "", i18n.T("cli.fl_template_file"))
	f.StringVar(&name, "name", "", i18n.T("cli.fl_name"))
	f.StringVar(&phone, "phone-col", "", i18n.T("cli.fl_phone_col"))
	f.StringVar(&mode, "mode", "", i18n.T("cli.fl_mode"))
	f.Float64Var(&rate, "rate", 3, i18n.T("cli.fl_rate"))
	f.StringVar(&sender, "sender", "", i18n.T("cli.fl_sender"))
	f.StringVar(&at, "at", "", i18n.T("cli.fl_at_camp"))
	f.BoolVar(&stop, "stop", false, i18n.T("cli.fl_stop"))
	f.BoolVarP(&yes, "yes", "y", false, i18n.T("cli.fl_yes"))
	f.BoolVar(&dry, "dry-run", false, i18n.T("cli.fl_dry_camp"))
	f.BoolVar(&wait, "wait", false, i18n.T("cli.fl_wait"))
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
			return 0, usagef("%s", i18n.T("cli.col_not_detected", i18n.P{"headers": strings.Join(info.Headers, ", ")}))
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
	return 0, usagef("%s", i18n.T("cli.col_not_found", i18n.P{"spec": fmt.Sprintf("%q", spec), "headers": strings.Join(info.Headers, ", ")}))
}

func printReport(info service.ImportInfo, r service.PlanReport) {
	w := os.Stderr
	fmt.Fprintln(w, i18n.T("cli.r_file", i18n.P{"file": info.File, "count": info.Rows}))
	fmt.Fprint(w, i18n.T("cli.r_recipients", i18n.P{"count": r.Recipients}))
	if r.InvalidCount > 0 {
		fmt.Fprint(w, i18n.T("cli.r_invalid", i18n.P{"count": r.InvalidCount}))
	}
	if r.Dupes > 0 {
		fmt.Fprint(w, i18n.T("cli.r_dupes", i18n.P{"count": r.Dupes}))
	}
	fmt.Fprint(w, "\n"+i18n.T("cli.r_mode", i18n.P{"mode": r.Mode}))
	if len(r.Variables) > 0 {
		fmt.Fprint(w, i18n.T("cli.r_variables", i18n.P{"names": strings.Join(r.Variables, ", ")}))
	}
	fmt.Fprintln(w, "\n"+i18n.T("cli.r_segments", i18n.P{"total": r.TotalParts, "max": r.MaxParts}))
	if r.TotalCredits > 0 {
		fmt.Fprintln(w, i18n.T("cli.r_cost", i18n.P{"credits": r.TotalCredits, "price": euro(r.PriceMin, r.PriceMax, r.Currency)}))
	}
	if r.DurationSeconds >= 1 {
		fmt.Fprintln(w, i18n.T("cli.r_duration", i18n.P{"d": time.Duration(r.DurationSeconds * float64(time.Second)).Round(time.Second)}))
	}
	for _, s := range r.Samples[:min(len(r.Samples), 2)] {
		fmt.Fprintln(w, i18n.T("cli.r_sample", i18n.P{"phone": s.Phone, "msg": trunc(s.Message, 80)}))
	}
	for _, i := range r.Invalid[:min(len(r.Invalid), 5)] {
		fmt.Fprintln(w, i18n.T("cli.r_ignored", i18n.P{"line": i.Line, "value": i.Value}))
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
		line := i18n.T("cli.follow_line", i18n.P{"name": c.Name, "sent": c.Sent, "total": c.Total, "failed": c.Failed, "status": campStatusLabel(c.Status)})
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
				return final, failf("%s", i18n.T("cli.follow_interrupted", i18n.P{"id": c.ID}))
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
		fmt.Println(i18n.T("cli.finish_line", i18n.P{"name": c.Name, "status": campStatusLabel(c.Status), "sent": c.Sent, "total": c.Total, "failed": c.Failed, "credits": c.Credits}))
		if c.Error != "" {
			fmt.Fprintln(os.Stderr, "  ! "+c.Error)
		}
	}
	switch {
	case c.Status == store.StatusPaused:
		return failf("%s", i18n.T("cli.finish_paused", i18n.P{"detail": orDefault(c.Error, i18n.T("cli.see_details")), "id": c.ID}))
	case c.Status == store.StatusFailed:
		return failf("%s", i18n.T("cli.finish_failed", i18n.P{"error": c.Error}))
	case c.Failed > 0:
		return failf("%s", i18n.T("cli.finish_n_failed", i18n.P{"count": c.Failed, "id": c.ID}))
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
		Short: i18n.T("cli.camp_ls_short"),
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
				fmt.Fprintln(tw, i18n.T("cli.th_camp"))
				for _, c := range list {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d/%d\t%d\t%v\t%s\n", c.ID, trunc(c.Name, 30), c.Mode,
						campStatusLabel(c.Status), c.Sent, c.Total, c.Failed, c.Credits, dateFr(c.CreatedAt))
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
		Short: i18n.T("cli.show_short"),
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
				fmt.Print(i18n.T("cli.show_a", i18n.P{"name": camp.Name, "id": camp.ID, "mode": camp.Mode, "status": campStatusLabel(camp.Status)}))
				fmt.Print(i18n.T("cli.show_b", i18n.P{"sent": camp.Sent, "total": camp.Total, "failed": camp.Failed, "credits": camp.Credits, "created": dateFr(camp.CreatedAt)}))
				if camp.SendAt != nil {
					fmt.Print(i18n.T("cli.show_sendat", i18n.P{"date": dateFr(*camp.SendAt)}))
				}
				fmt.Print(i18n.T("cli.show_msg", i18n.P{"v": trunc(camp.Template, 120)}))
				if camp.Error != "" {
					fmt.Print(i18n.T("cli.show_err", i18n.P{"v": camp.Error}))
				}
				if recipients {
					tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
					fmt.Fprintln(tw, i18n.T("cli.th_recipients"))
					for _, r := range page.Items {
						fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", r.Idx+1, r.Phone, r.Status, trunc(r.Error, 80))
					}
					_ = tw.Flush()
					fmt.Fprintln(os.Stderr, i18n.T("cli.show_footer", i18n.P{"shown": len(page.Items), "total": page.Total}))
				}
				return nil
			})
		},
	}
	c.Flags().BoolVar(&recipients, "recipients", false, i18n.T("cli.fl_recipients"))
	c.Flags().StringVar(&status, "status", "", i18n.T("cli.fl_status_filter"))
	c.Flags().IntVarP(&limit, "limit", "n", 50, i18n.T("cli.fl_limit_recipients"))
	return c
}

// ---------- reprise / annulation / relance / suppression ----------

func campResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <id>",
		Short: i18n.T("cli.resume_short"),
		Args:  exactly(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(ctx context.Context, svc *service.Service) error {
				camp, err := svc.GetCampaign(ctx, args[0])
				if err != nil {
					return err
				}
				if camp.Mode != store.ModeLocal {
					return failf("%s", i18n.T("cli.resume_local_only"))
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
		Short: i18n.T("cli.retry_short"),
		Args:  exactly(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(ctx context.Context, svc *service.Service) error {
				n, err := svc.RetryFailed(ctx, args[0])
				if err != nil {
					return err
				}
				if n == 0 {
					fmt.Println(i18n.T("cli.retry_none"))
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
		Short: i18n.T("cli.cancel_short"),
		Args:  exactly(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := confirm(i18n.T("cli.cancel_confirm"), yes); err != nil {
				return err
			}
			return withService(func(ctx context.Context, svc *service.Service) error {
				if err := svc.CancelCampaign(ctx, args[0]); err != nil {
					return err
				}
				if asJSON {
					printJSON(os.Stdout, map[string]string{"canceled": args[0]})
				} else {
					fmt.Println(i18n.T("cli.cancel_done"))
				}
				return nil
			})
		},
	}
	c.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("cli.fl_yes"))
	return c
}

func campRmCmd() *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:   "rm <id>",
		Short: i18n.T("cli.crm_short"),
		Args:  exactly(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := confirm(i18n.T("cli.crm_confirm"), yes); err != nil {
				return err
			}
			return withService(func(ctx context.Context, svc *service.Service) error {
				if err := svc.DeleteCampaign(ctx, args[0]); err != nil {
					return err
				}
				if asJSON {
					printJSON(os.Stdout, map[string]string{"deleted": args[0]})
				} else {
					fmt.Println(i18n.T("cli.crm_done"))
				}
				return nil
			})
		},
	}
	c.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("cli.fl_yes"))
	return c
}
