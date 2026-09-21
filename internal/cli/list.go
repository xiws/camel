package cli

import (
	"context"
	"fmt"

	"camel/internal/credential"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list [provider] [dir]",
	Short: "List files in directory",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		providerName := args[0]
		dir := "/"
		if len(args) > 1 {
			dir = args[1]
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

		files, err := p.List(context.Background(), dir)
		if err != nil {
			return err
		}

		if len(files) == 0 {
			fmt.Println("Directory is empty")
			return nil
		}

		for _, f := range files {
			kind := "FILE"
			if f.IsDir {
				kind = "DIR "
			}
			size := ""
			if !f.IsDir {
				size = fmt.Sprintf("  %d bytes", f.Size)
			}
			fmt.Printf("[%s] %s%s\n", kind, f.Name, size)
		}

		return nil
	},
}
