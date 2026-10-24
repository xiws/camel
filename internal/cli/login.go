package cli

import (
	"context"
	"fmt"

	"camel/internal/credential"
	"camel/internal/provider"

	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login [provider]",
	Short: "Login to cloud storage provider",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		providerName := args[0]
		p, err := getProvider(providerName)
		if err != nil {
			return err
		}

		username, _ := cmd.Flags().GetString("username")
		password, _ := cmd.Flags().GetString("password")
		bduss, _ := cmd.Flags().GetString("bduss")
		token, _ := cmd.Flags().GetString("token")

		params := provider.LoginParams{
			Username:    username,
			Password:    password,
			BDUSS:       bduss,
			AccessToken: token,
		}

		creds, err := p.Login(context.Background(), params)
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}

		if err := credential.Save(providerName, creds); err != nil {
			return fmt.Errorf("failed to save credentials: %w", err)
		}

		fmt.Printf("Login successful. Credentials saved to ~/.camel/%s.enc\n", providerName)
		return nil
	},
}

func init() {
	loginCmd.Flags().StringP("username", "u", "", "Username or phone number")
	loginCmd.Flags().StringP("password", "p", "", "Password")
	loginCmd.Flags().String("bduss", "", "BDUSS token (Baidu only)")
	loginCmd.Flags().String("token", "", "Access token (联通沃云盘，直接传入已有 token 跳过登录流程)")
}
