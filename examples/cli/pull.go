package main

import (
	"context"
	"fmt"

	"github.com/acheong08/tangled-go-sdk"
	"github.com/spf13/cobra"
)

func pullCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Manage pull requests",
	}

	cmd.AddCommand(pullCreateCmd())
	cmd.AddCommand(pullShowCmd())

	return cmd
}

func pullCreateCmd() *cobra.Command {
	var title, body, sourceBranch, targetBranch string

	cmd := &cobra.Command{
		Use:   "create <owner/repo>",
		Short: "Create a new pull request",
		Args:  cobra.ExactArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClient(context.Background())
			if err != nil {
				return err
			}

			if title == "" {
				return fmt.Errorf("title is required (use --title)")
			}
			if sourceBranch == "" {
				return fmt.Errorf("source branch is required (use --source)")
			}

			pull, err := client.CreatePull(context.Background(), args[0], tangled.CreatePullParams{
				Title:        title,
				Body:         body,
				SourceBranch: sourceBranch,
				TargetBranch: targetBranch,
			})
			if err != nil {
				return err
			}

			fmt.Printf("Created PR #%d: %s\n", pull.ID, pull.Title)
			return nil
		},
	}

	cmd.Flags().StringVarP(&title, "title", "t", "", "PR title (required)")
	cmd.Flags().StringVarP(&body, "body", "b", "", "PR body")
	cmd.Flags().StringVarP(&sourceBranch, "source", "s", "", "Source branch (required)")
	cmd.Flags().StringVarP(&targetBranch, "target", "T", "main", "Target branch")

	return cmd
}

func pullShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <owner/repo> <pull-id>",
		Short: "Show pull request details",
		Args:  cobra.ExactArgs(2),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClient(context.Background())
			if err != nil {
				return err
			}

			var pullID int
			if _, err := fmt.Sscanf(args[1], "%d", &pullID); err != nil {
				return fmt.Errorf("invalid pull ID %q: %w", args[1], err)
			}

			pulls, err := client.ListPulls(context.Background(), args[0], 100)
			if err != nil {
				return err
			}

			for _, p := range pulls {
				if p.ID == pullID {
					fmt.Printf("PR #%d: %s\n", p.ID, p.Title)
					if p.Body != "" {
						fmt.Println()
						fmt.Println(p.Body)
					}
					fmt.Printf("\n%s -> %s\n", p.Source.Branch, p.Target.Branch)
					fmt.Printf("Created: %s\n", p.CreatedAt)
					fmt.Printf("URI: %s\n", p.URI)
					return nil
				}
			}

			return fmt.Errorf("pull request #%d not found", pullID)
		},
	}
}
