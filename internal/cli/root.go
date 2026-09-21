package cli

import (
	"fmt"
	"os"

	"camel/internal/provider"
	"camel/internal/provider/baidu"
	"camel/internal/provider/liantong"

	"github.com/spf13/cobra"
)

var providers = map[string]provider.Provider{
	"baidu": baidu.New(),
	"lt":    liantong.New(),
}

var rootCmd = &cobra.Command{
	Use:   "camel",
	Short: "Cloud storage CLI for Baidu Netdisk and Liantong WoCloud",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(uploadCmd)
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(delCmd)
	rootCmd.AddCommand(mvCmd)
	rootCmd.AddCommand(cpCmd)
	rootCmd.AddCommand(touchCmd)
	rootCmd.AddCommand(mkdirCmd)
}

func getProvider(name string) (provider.Provider, error) {
	p, ok := providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
	return p, nil
}
