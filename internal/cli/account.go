package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"smsh/internal/i18n"
	"smsh/internal/service"
)

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: i18n.T("cli.status_short"),
		Args:  exactly(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withService(func(ctx context.Context, svc *service.Service) error {
				cctx, cancel := context.WithTimeout(ctx, 30e9)
				defer cancel()
				ov, err := svc.Overview(cctx)
				if err != nil {
					return err
				}
				if asJSON {
					printJSON(os.Stdout, ov)
					return nil
				}
				fmt.Println(i18n.T("cli.l_account", i18n.P{"name": ov.Account, "service": ov.ServiceName}))
				fmt.Println(i18n.T("cli.l_state", i18n.P{"v": ov.Status}))
				fmt.Println(i18n.T("cli.l_credits", i18n.P{"v": ov.CreditsLeft}))
				names := make([]string, 0, len(ov.Senders))
				for _, s := range ov.Senders {
					names = append(names, fmt.Sprintf("%s (%s)", s.Name, s.Type))
				}
				if len(names) > 0 {
					fmt.Println(i18n.T("cli.l_senders", i18n.P{"v": strings.Join(names, ", ")}))
				}
				return nil
			})
		},
	}
}

func accountCmd() *cobra.Command {
	c := &cobra.Command{Use: "account", Short: i18n.T("cli.account_short")}
	c.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: i18n.T("cli.account_ls_short"),
		Args:  exactly(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withService(func(_ context.Context, svc *service.Service) error {
				views, err := svc.Creds.List()
				if err != nil {
					return err
				}
				if asJSON {
					printJSON(os.Stdout, views)
					return nil
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
				fmt.Fprintln(tw, i18n.T("cli.th_account"))
				for _, v := range views {
					mark := ""
					if v.Active {
						mark = "*"
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", mark, v.ID, v.Name, v.ServiceName, v.Endpoint)
				}
				return tw.Flush()
			})
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   i18n.T("cli.account_use_usage"),
		Short: i18n.T("cli.account_use_short"),
		Args:  exactly(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(_ context.Context, svc *service.Service) error {
				views, err := svc.Creds.List()
				if err != nil {
					return err
				}
				for _, v := range views {
					if v.ID == args[0] || strings.EqualFold(v.Name, args[0]) {
						if err := svc.Creds.SetActive(v.ID); err != nil {
							return err
						}
						if asJSON {
							printJSON(os.Stdout, map[string]string{"active": v.ID, "name": v.Name})
						} else {
							fmt.Println(i18n.T("cli.account_active", i18n.P{"name": v.Name, "service": v.ServiceName}))
						}
						return nil
					}
				}
				return failf("%s", i18n.T("cli.account_unknown", i18n.P{"name": args[0]}))
			})
		},
	})
	return c
}
