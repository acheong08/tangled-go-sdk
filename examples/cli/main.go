package main

import (
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "tangled",
		Short: "CLI for the Tangled code collaboration platform",
		Long:  "A command-line client for Tangled, a decentralized git hosting platform built on the AT Protocol.",
	}

	rootCmd.AddCommand(loginCmd())
	rootCmd.AddCommand(whoamiCmd())
	rootCmd.AddCommand(listCmd())
	rootCmd.AddCommand(issueCmd())
	rootCmd.AddCommand(pullCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
