package main

import (
	"context"
	"fmt"

	"github.com/acheong08/tangled-go-sdk"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// newClient creates an authenticated client from saved config.
// Returns an error if not logged in.
func newClient(ctx context.Context) (*tangled.Client, error) {
	handle := viper.GetString("handle")
	password := viper.GetString("password")
	if handle == "" || password == "" {
		return nil, fmt.Errorf("not logged in — run `tangled login` first")
	}
	return tangled.NewClient(ctx, tangled.Config{
		Handle:   handle,
		Password: password,
	})
}

func whoamiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show the currently logged-in identity",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClient(context.Background())
			if err != nil {
				return err
			}
			fmt.Printf("%s (%s)\n", client.Handle(), client.DID())
			return nil
		},
	}

	return cmd
}
