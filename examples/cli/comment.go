package main

import (
	"context"
	"fmt"

	"github.com/acheong08/tangled-go-sdk"
	"github.com/spf13/cobra"
)

func commentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comment",
		Short: "Comment on an issue or pull request",
	}

	cmd.AddCommand(commentCreateCmd())
	cmd.AddCommand(commentListCmd())

	return cmd
}

func commentCreateCmd() *cobra.Command {
	var body string

	cmd := &cobra.Command{
		Use:   "create <owner/repo> <issue-or-pr-id>",
		Short: "Create a comment on an issue or pull request",
		Long: `Create a comment on an issue or pull request.

The ID refers to the issue or PR number (e.g., #1, #42).
The command first looks up the issue or PR to get its AT-URI and CID,
then creates the comment record on your PDS.`,
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
				return fmt.Errorf("body is required (use --body)")
			}

			var subjectURI, subjectCID string

			// Try issue first, then PR
			var issueID int
			if _, err := fmt.Sscanf(args[1], "%d", &issueID); err != nil {
				return fmt.Errorf("invalid ID %q: %w", args[1], err)
			}

			issue, err := client.GetIssue(context.Background(), args[0], issueID)
			if err == nil && issue != nil {
				subjectURI = issue.URI
				subjectCID = issue.CID
			} else {
				// Try PRs
				pulls, err := client.ListPulls(context.Background(), args[0], 100)
				if err != nil {
					return fmt.Errorf("failed to find issue or PR #%d: %w", issueID, err)
				}
				found := false
				for _, p := range pulls {
					if p.ID == issueID {
						subjectURI = p.URI
						subjectCID = p.CID
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("issue or PR #%d not found", issueID)
				}
			}

			comment, err := client.CreateComment(context.Background(), tangled.CreateCommentParams{
				Body:        body,
				SubjectURI:  subjectURI,
				SubjectCID:  subjectCID,
			})
			if err != nil {
				return err
			}

			fmt.Printf("Comment created: %s\n", comment.URI)
			return nil
		},
	}

	cmd.Flags().StringVarP(&body, "body", "m", "", "Comment body (required)")

	return cmd
}

func commentListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <owner/repo> [subject-uri]",
		Short: "List comments on a repository",
		Long: `List comments on a repository.

If a subject URI is provided (e.g., an issue AT-URI), only comments
on that subject are returned. Otherwise, all comments from the repo
owner's PDS are listed.`,
		Args: cobra.RangeArgs(1, 2),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client := tangled.NewPublicClient()

			subjectURI := ""
			if len(args) >= 2 {
				subjectURI = args[1]
			}

			comments, err := client.ListComments(context.Background(), args[0], subjectURI, 50)
			if err != nil {
				return err
			}

			if len(comments) == 0 {
				fmt.Println("No comments found.")
				return nil
			}

			for _, c := range comments {
				fmt.Printf("%s\n", c.Body)
				fmt.Printf("  by %s at %s\n", c.URI, c.CreatedAt)
				fmt.Println()
			}
			return nil
		},
	}
}
