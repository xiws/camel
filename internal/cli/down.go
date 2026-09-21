package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"camel/internal/credential"
	"camel/internal/progress"
	"camel/internal/provider"

	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down [provider] [remotePath]",
	Short: "Download file from cloud storage",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		providerName := args[0]
		remotePath := args[1]

		localPath, _ := cmd.Flags().GetString("output")
		if localPath == "" {
			localPath = filepath.Base(remotePath)
		}

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

		bar := progress.New(fmt.Sprintf("Downloading %s", filepath.Base(remotePath)), -1)
		ctx := provider.WithProgress(context.Background(), func(transferred, total int64) {
			if total > 0 {
				bar.SetTotal(total)
			}
			bar.Set(transferred)
		})

		if err := p.Download(ctx, remotePath, localPath); err != nil {
			return err
		}

		bar.Finish()
		fmt.Printf("Saved to %s\n", localPath)
		return nil
	},
}

func init() {
	downCmd.Flags().StringP("output", "o", "", "Local file path (default: filename from remotePath)")
}
