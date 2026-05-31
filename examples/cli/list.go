package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/acheong08/tangled-go-sdk"
	"github.com/spf13/cobra"
)

func listCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List resources (repos, branches, issues, pulls)",
	}

	cmd.AddCommand(listReposCmd())
	cmd.AddCommand(listBranchesCmd())
	cmd.AddCommand(listIssuesCmd())
	cmd.AddCommand(listPullsCmd())
	cmd.AddCommand(listLabelsCmd())

	return cmd
}

func listReposCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "repos",
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

func listBranchesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "branches <owner/repo>",
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

func listIssuesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "issues <owner/repo>",
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

			tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tTITLE\tCREATED")
			for _, i := range issues {
				fmt.Fprintf(tw, "#%d\t%s\t%s\n", i.ID, i.Title, i.CreatedAt)
			}
			tw.Flush()
			return nil
		},
	}
}

func listPullsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pulls <owner/repo>",
		Short: "List pull requests on a repository",
		Args:  cobra.ExactArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client := tangled.NewPublicClient()

			pulls, err := client.ListPulls(context.Background(), args[0], 50)
			if err != nil {
				return err
			}

			if len(pulls) == 0 {
				fmt.Println("No pull requests found.")
				return nil
			}

			tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tTITLE\tSOURCE\tTARGET\tCREATED")
			for _, p := range pulls {
				fmt.Fprintf(tw, "#%d\t%s\t%s\t%s\t%s\n",
					p.ID, p.Title, p.Source.Branch, p.Target.Branch, p.CreatedAt)
			}
			tw.Flush()
			return nil
		},
	}
}

func listLabelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "labels <owner/repo>",
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
