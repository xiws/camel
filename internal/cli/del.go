package cli

import (
	"context"
	"fmt"

	"camel/internal/credential"

	"github.com/spf13/cobra"
)

var delCmd = &cobra.Command{
	Use:   "del [provider] [paths...]",
	Short: "Delete files from cloud storage",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		providerName := args[0]
		paths := args[1:]

		p, err := getProvider(providerName)
		if err != nil {
			return err
		}

		creds, err := credential.Load(providerName)
		if err != nil {
			return fmt.Errorf("not logged in. Run 'camel login %s' first", providerName)
		}

		if err := p.Init(context.Background(), creds); err != nil {
			return fmt.Errorf("failed to initialize provider: %w", err)
		}

		if err := p.Delete(context.Background(), paths); err != nil {
			return err
		}

		fmt.Printf("Deleted %d item(s)\n", len(paths))
		return nil
	},
}
