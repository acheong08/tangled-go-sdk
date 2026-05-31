package main

import (
	"context"
	"fmt"

	"github.com/acheong08/tangled-go-sdk"
	"github.com/spf13/cobra"
)

func issueCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue",
		Short: "Manage issues",
	}

	cmd.AddCommand(issueCreateCmd())
	cmd.AddCommand(issueShowCmd())
	cmd.AddCommand(issueCloseCmd())

	return cmd
}

func issueCreateCmd() *cobra.Command {
	var title, body string
	var labels []string

	cmd := &cobra.Command{
		Use:   "create <owner/repo>",
		Short: "Create a new issue",
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

			issue, err := client.CreateIssue(context.Background(), args[0], tangled.CreateIssueParams{
				Title:  title,
				Body:   body,
				Labels: labels,
			})
			if err != nil {
				return err
			}

			fmt.Printf("Created issue #%d: %s\n", issue.ID, issue.Title)
			return nil
		},
	}

	cmd.Flags().StringVarP(&title, "title", "t", "", "Issue title (required)")
	cmd.Flags().StringVarP(&body, "body", "b", "", "Issue body")
	cmd.Flags().StringSliceVarP(&labels, "label", "l", nil, "Labels (comma-separated or repeated)")

	return cmd
}

func issueShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <owner/repo> <issue-id>",
		Short: "Show issue details",
		Args:  cobra.ExactArgs(2),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClient(context.Background())
			if err != nil {
				return err
			}

			var issueID int
			if _, err := fmt.Sscanf(args[1], "%d", &issueID); err != nil {
				return fmt.Errorf("invalid issue ID %q: %w", args[1], err)
			}

			issue, err := client.GetIssue(context.Background(), args[0], issueID)
			if err != nil {
				return err
			}

			fmt.Printf("Issue #%d: %s\n", issue.ID, issue.Title)
			if issue.Body != "" {
				fmt.Println()
				fmt.Println(issue.Body)
			}
			if len(issue.Labels) > 0 {
				fmt.Printf("\nLabels: %v\n", issue.Labels)
			}
			fmt.Printf("\nCreated: %s\n", issue.CreatedAt)
			fmt.Printf("URI: %s\n", issue.URI)
			return nil
		},
	}
}

func issueCloseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "close <owner/repo> <issue-id>",
		Short: "Delete an issue",
		Args:  cobra.ExactArgs(2),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClient(context.Background())
			if err != nil {
				return err
			}

			var issueID int
			if _, err := fmt.Sscanf(args[1], "%d", &issueID); err != nil {
				return fmt.Errorf("invalid issue ID %q: %w", args[1], err)
			}

			if err := client.DeleteIssue(context.Background(), args[0], issueID); err != nil {
				return err
			}

			fmt.Printf("Deleted issue #%d\n", issueID)
			return nil
		},
	}
}
