package cli

import (
	"context"
	"fmt"

	"camel/internal/credential"

	"github.com/spf13/cobra"
)

var mkdirCmd = &cobra.Command{
	Use:   "mkdir [provider] [path]",
	Short: "Create a directory on cloud storage",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		providerName := args[0]
		dirPath := args[1]

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

		if err := p.Mkdir(context.Background(), dirPath); err != nil {
			return err
		}

		fmt.Printf("Created directory %s\n", dirPath)
		return nil
	},
}
