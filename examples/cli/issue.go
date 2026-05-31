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

	cmd.AddCommand(issueListCmd())
	cmd.AddCommand(issueCreateCmd())
	cmd.AddCommand(issueShowCmd())
	cmd.AddCommand(issueCloseCmd())
	cmd.AddCommand(issueReopenCmd())
	cmd.AddCommand(issueCommentCmd())

	return cmd
}

func issueListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <owner/repo>",
		Short: "List issues on a repository",
		Args:  cobra.ExactArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client := tangled.NewPublicClient()

			issues, err := client.ListIssues(context.Background(), args[0], 50)
			if err != nil {
				return err
			}

			if len(issues) == 0 {
				fmt.Println("No issues found.")
				return nil
			}

			for _, i := range issues {
				fmt.Printf("#%d\t[%s]\t%s\t%s\n", i.ID, shortState(i.State), i.Title, i.CreatedAt)
			}
			return nil
		},
	}
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

			fmt.Printf("Issue #%d [%s]: %s\n", issue.ID, shortState(issue.State), issue.Title)
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
		Short: "Close an issue",
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

			if err := client.CloseIssue(context.Background(), args[0], issueID); err != nil {
				return err
			}

			fmt.Printf("Closed issue #%d\n", issueID)
			return nil
		},
	}
}

func issueReopenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reopen <owner/repo> <issue-id>",
		Short: "Reopen a closed issue",
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

			if err := client.ReopenIssue(context.Background(), args[0], issueID); err != nil {
				return err
			}

			fmt.Printf("Reopened issue #%d\n", issueID)
			return nil
		},
	}
}

func issueCommentCmd() *cobra.Command {
	var body string

	cmd := &cobra.Command{
		Use:   "comment <owner/repo> <issue-id>",
		Short: "Comment on an issue",
		Args:  cobra.ExactArgs(2),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClient(context.Background())
			if err != nil {
				return err
			}

			if body == "" {
				return fmt.Errorf("body is required (use --body or -m)")
			}

			comment, err := client.CreateComment(context.Background(), tangled.CreateCommentParams{
				Body:           body,
				OwnerSlashRepo: args[0],
				SubjectRef:     args[1],
			})
			if err != nil {
				return err
			}

			fmt.Printf("Comment created on issue #%s: %s\n", args[1], comment.URI)
			return nil
		},
	}

	cmd.Flags().StringVarP(&body, "body", "m", "", "Comment body (required)")

	return cmd
}

// shortState returns a short human-readable label for an issue state.
func shortState(state string) string {
	switch state {
	case tangled.IssueStateClosed:
		return "closed"
	case tangled.IssueStateOpen:
		return "open"
	default:
		if state == "" {
			return "open"
		}
		return state
	}
}
