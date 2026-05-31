package main

import (
	"context"
	"fmt"
	"os"
	"syscall"

	"github.com/acheong08/tangled-go-sdk"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

func loginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with your AT Protocol handle and app password",
		Long: `Login saves your AT Protocol handle and app password to the config file.

Create an app password at https://bsky.app/settings/app-passwords

The credentials are stored in plaintext in your config directory:
  Linux:   ~/.config/tangled/config.toml
  macOS:   ~/Library/Application Support/tangled/config.toml
  Windows: %AppData%\tangled\config.toml`,
		Args: cobra.MaximumNArgs(2),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			handle := ""
			password := ""

			if len(args) >= 1 {
				handle = args[0]
			} else {
				fmt.Print("Handle (e.g. user.bsky.social): ")
				fmt.Scanln(&handle)
			}

			if len(args) >= 2 {
				password = args[1]
			} else {
				fmt.Print("App password (hidden): ")
				pwdBytes, err := term.ReadPassword(int(syscall.Stdin))
				fmt.Println() // newline after hidden input
				if err != nil {
					return fmt.Errorf("failed to read password: %w", err)
				}
				password = string(pwdBytes)
			}

			if handle == "" || password == "" {
				return fmt.Errorf("handle and password are required")
			}

			// Validate credentials by creating a client
			fmt.Fprintf(os.Stderr, "Verifying credentials...\n")
			client, err := tangled.NewClient(context.Background(), tangled.Config{
				Handle:   handle,
				Password: password,
			})
			if err != nil {
				return fmt.Errorf("authentication failed: %w", err)
			}

			// Save to config
			viper.Set("handle", handle)
			viper.Set("password", password)

			if err := saveConfig(); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Logged in as %s (%s)\n", client.Handle(), client.DID())
			return nil
		},
	}

	return cmd
}
