package cli

import (
	"context"
	"fmt"
	"path"

	"camel/internal/credential"

	"github.com/spf13/cobra"
)

var cpCmd = &cobra.Command{
	Use:   "cp [provider] [src] [dest]",
	Short: "Copy files on cloud storage",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		providerName := args[0]
		src := args[1]
		dest := args[2]

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

		destDir := path.Dir(dest)
		newName := path.Base(dest)

		if destDir == "." {
			destDir = path.Dir(src)
		}

		if err := p.Copy(context.Background(), src, destDir, newName); err != nil {
			return err
		}

		fmt.Printf("Copied %s -> %s/%s\n", src, destDir, newName)
		return nil
	},
}
