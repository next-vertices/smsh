package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"smsh/internal/service"
)

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Teste la connexion et affiche le solde et les expéditeurs du compte actif",
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
				fmt.Printf("Compte        : %s (%s)\n", ov.Account, ov.ServiceName)
				fmt.Printf("État          : %s\n", ov.Status)
				fmt.Printf("Crédits       : %v\n", ov.CreditsLeft)
				names := make([]string, 0, len(ov.Senders))
				for _, s := range ov.Senders {
					names = append(names, fmt.Sprintf("%s (%s)", s.Name, s.Type))
				}
				if len(names) > 0 {
					fmt.Printf("Expéditeurs   : %s\n", strings.Join(names, ", "))
				}
				return nil
			})
		},
	}
}

func accountCmd() *cobra.Command {
	c := &cobra.Command{Use: "account", Short: "Comptes OVH enregistrés (ajout et modification dans l'application)"}
	c.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "Liste les comptes (secrets masqués)",
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
				fmt.Fprintln(tw, "ACTIF\tID\tNOM\tSERVICE\tENDPOINT")
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
		Use:   "use <id|nom>",
		Short: "Change le compte actif (partagé avec l'application)",
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
							fmt.Printf("Compte actif : %s (%s)\n", v.Name, v.ServiceName)
						}
						return nil
					}
				}
				return failf("compte introuvable : %s (voir « smsh account ls »)", args[0])
			})
		},
	})
	return c
}
