package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/acheong08/tangled-go-sdk"
	"github.com/spf13/cobra"
)

func branchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "branch",
		Short: "Manage branches",
	}

	cmd.AddCommand(branchListCmd())

	return cmd
}

func branchListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <owner/repo>",
		Short: "List branches on a repository",
		Args:  cobra.ExactArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client := tangled.NewPublicClient()

			branches, err := client.ListBranches(context.Background(), args[0], 100)
			if err != nil {
				return err
			}

			if len(branches) == 0 {
				fmt.Println("No branches found.")
				return nil
			}

			tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "BRANCH\tSHA")
			for _, b := range branches {
				fmt.Fprintf(tw, "%s\t%s\n", b.Name, b.SHA)
			}
			tw.Flush()
			return nil
		},
	}
}
