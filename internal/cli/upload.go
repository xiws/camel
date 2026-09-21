package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"camel/internal/credential"
	"camel/internal/progress"
	"camel/internal/provider"

	"github.com/spf13/cobra"
)

var uploadCmd = &cobra.Command{
	Use:   "upload [provider] [remoteDir] [files...]",
	Short: "Upload files to cloud storage",
	Args:  cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		providerName := args[0]
		remoteDir := args[1]
		filePatterns := args[2:]

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

		var files []string
		for _, pattern := range filePatterns {
			matches, err := filepath.Glob(pattern)
			if err != nil {
				return fmt.Errorf("invalid glob pattern %s: %w", pattern, err)
			}
			if len(matches) == 0 {
				files = append(files, pattern)
			} else {
				files = append(files, matches...)
			}
		}

		if len(files) == 0 {
			return fmt.Errorf("no files to upload")
		}

		for _, file := range files {
			info, err := os.Stat(file)
			if err != nil {
				return fmt.Errorf("cannot access %s: %w", file, err)
			}

			bar := progress.New(fmt.Sprintf("Uploading %s", filepath.Base(file)), info.Size())
			ctx := provider.WithProgress(context.Background(), func(transferred, total int64) {
				bar.Set(transferred)
			})

			if err := p.Upload(ctx, file, remoteDir); err != nil {
				return fmt.Errorf("failed to upload %s: %w", file, err)
			}
			bar.Finish()
		}

		return nil
	},
}
