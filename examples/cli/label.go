package main

import (
	"context"
	"fmt"

	"github.com/acheong08/tangled-go-sdk"
	"github.com/spf13/cobra"
)

func labelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "label",
		Short: "Manage labels",
	}

	cmd.AddCommand(labelListCmd())

	return cmd
}

func labelListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <owner/repo>",
		Short: "List available labels for a repository",
		Args:  cobra.ExactArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client := tangled.NewPublicClient()

			labels, err := client.ListLabels(context.Background(), args[0])
			if err != nil {
				return err
			}

			if len(labels) == 0 {
				fmt.Println("No labels found.")
				return nil
			}

			for _, l := range labels {
				fmt.Println(l)
			}
			return nil
		},
	}
}
