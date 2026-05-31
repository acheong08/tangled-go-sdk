package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/acheong08/tangled-go-sdk"
	"github.com/spf13/cobra"
)

func commentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comment",
		Short: "Comment on issues and pull requests",
	}

	cmd.AddCommand(commentCreateCmd())
	cmd.AddCommand(commentListCmd())

	return cmd
}

func commentCreateCmd() *cobra.Command {
	var body string

	cmd := &cobra.Command{
		Use:   "create <owner/repo> <issue-or-pr-id>",
		Short: "Comment on an issue or pull request",
		Long: `Comment on an issue or pull request.

The ID refers to the issue or PR number (e.g., 1, 42).
The command resolves the issue/PR automatically.`,
		Args: cobra.ExactArgs(2),
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

			var id int
			if _, err := fmt.Sscanf(args[1], "%d", &id); err != nil {
				return fmt.Errorf("invalid ID %q: %w", args[1], err)
			}

			comment, err := client.CreateComment(context.Background(), tangled.CreateCommentParams{
				Body:           body,
				OwnerSlashRepo: args[0],
				IssueID:        id,
			})
			if err != nil {
				return err
			}

			fmt.Printf("Comment created on #%d: %s\n", id, comment.URI)
			return nil
		},
	}

	cmd.Flags().StringVarP(&body, "body", "m", "", "Comment body (required)")

	return cmd
}

func commentListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <owner/repo> [issue-or-pr-id]",
		Short: "List comments",
		Long: `List comments on a repository.

If an issue/PR ID is given, only comments on that issue or PR are shown.
Otherwise, all comments from the repo owner's PDS are listed.`,
		Args: cobra.RangeArgs(1, 2),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client := tangled.NewPublicClient()

			id := 0
			if len(args) >= 2 {
				if _, err := fmt.Sscanf(args[1], "%d", &id); err != nil {
					return fmt.Errorf("invalid ID %q: %w", args[1], err)
				}
			}

			comments, err := client.ListComments(context.Background(), args[0], id, 50)
			if err != nil {
				return err
			}

			if len(comments) == 0 {
				fmt.Println("No comments found.")
				return nil
			}

			tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			for _, c := range comments {
				fmt.Fprintf(tw, "%s\t%s\n", c.Body, c.CreatedAt)
			}
			tw.Flush()
			return nil
		},
	}
}
