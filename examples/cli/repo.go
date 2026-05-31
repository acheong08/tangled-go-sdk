package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func repoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage repositories",
	}

	cmd.AddCommand(repoListCmd())

	return cmd
}

func repoListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your repositories",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClient(context.Background())
			if err != nil {
				return err
			}

			repos, err := client.ListMyRepos(context.Background())
			if err != nil {
				return err
			}

			if len(repos) == 0 {
				fmt.Println("No repositories found.")
				return nil
			}

			tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tKNOT\tDESCRIPTION")
			for _, r := range repos {
				fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Name, r.Knot, r.Description)
			}
			tw.Flush()
			return nil
		},
	}
}
