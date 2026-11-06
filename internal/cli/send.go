package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"smsh/internal/i18n"
	"smsh/internal/service"
)

func sendCmd() *cobra.Command {
	var (
		message, file, sender, at string
		stop, yes, dry            bool
	)
	c := &cobra.Command{
		Use:     i18n.T("cli.send_usage"),
		Short:   i18n.T("cli.send_short"),
		Example: i18n.T("cli.send_example"),
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return usagef("%s", i18n.T("cli.send_need_number"))
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
					fmt.Fprintln(os.Stderr, i18n.T("cli.l_message", i18n.P{"v": trunc(msg, 100)}))
					fmt.Fprintln(os.Stderr, i18n.T("cli.l_recipients", i18n.P{"v": strings.Join(args, ", ")}))
					if estErr == nil && est.Recipients > 0 {
						fmt.Fprintln(os.Stderr, i18n.T("cli.l_segments", i18n.P{"parts": est.Parts, "encoding": est.Encoding, "chars": est.Characters}))
						fmt.Fprintln(os.Stderr, i18n.T("cli.l_cost", i18n.P{"credits": est.TotalCredits, "price": euro(est.PriceMin, est.PriceMax, est.Currency)}))
					} else if estErr != nil {
						fmt.Fprintln(os.Stderr, i18n.T("cli.l_cost_na", i18n.P{"error": service.Describe(estErr).Message}))
					}
					if in.SendAt != "" {
						fmt.Fprintln(os.Stderr, i18n.T("cli.l_scheduled", i18n.P{"v": in.SendAt}))
					}
				}
				if dry {
					if asJSON {
						printJSON(os.Stdout, map[string]any{"dryRun": true, "estimate": est})
					} else {
						fmt.Fprintln(os.Stderr, i18n.T("cli.dry_done"))
					}
					return nil
				}
				if err := confirm(i18n.T("cli.confirm_send"), yes); err != nil {
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
				verb := i18n.T("cli.verb_sent")
				if out.Scheduled {
					verb = i18n.T("cli.verb_scheduled")
				}
				fmt.Println(i18n.T("cli.sent_line", i18n.P{"count": len(out.Sent), "verb": verb, "numbers": strings.Join(out.Sent, ", "), "credits": out.CreditsRemoved}))
				if len(out.Rejected) > 0 {
					fmt.Println(i18n.T("cli.rejected_line", i18n.P{"numbers": strings.Join(out.Rejected, ", ")}))
					return failf("%s", i18n.T("cli.rejected_err", i18n.P{"count": len(out.Rejected)}))
				}
				return nil
			})
		},
	}
	f := c.Flags()
	f.StringVarP(&message, "message", "m", "", i18n.T("cli.fl_message"))
	f.StringVarP(&file, "file", "f", "", i18n.T("cli.fl_file"))
	f.StringVar(&sender, "sender", "", i18n.T("cli.fl_sender"))
	f.StringVar(&at, "at", "", i18n.T("cli.fl_at_send"))
	f.BoolVar(&stop, "stop", false, i18n.T("cli.fl_stop"))
	f.BoolVarP(&yes, "yes", "y", false, i18n.T("cli.fl_yes"))
	f.BoolVar(&dry, "dry-run", false, i18n.T("cli.fl_dry_send"))
	return c
}
