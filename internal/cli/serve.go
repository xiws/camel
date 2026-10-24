package cli

import (
	"fmt"
	"net/http"
	"strings"

	"camel/internal/credential"
	"camel/internal/provider/liantong"
	"camel/internal/provider/webdav"

	"github.com/spf13/cobra"
)

var (
	serveAddr     string
	serveUser     string
	serveToken    string
	serveTLSCert  string
	serveTLSKey   string
	serveProvider string
)

var serveCmd = &cobra.Command{
	Use:   "serve [provider]",
	Short: "Start a WebDAV server for cloud storage",
	Long: `Start an HTTP server that exposes the specified cloud storage provider
as a WebDAV endpoint. Any WebDAV client (rclone mount, FileZilla, Finder, etc.)
can connect to browse, upload, download, and manage files.

Examples:
  camel serve lt                     # Start on :8080 using saved credentials
  camel serve lt --addr :9000        # Custom port
  camel serve lt --token <token>     # Inline token (no login needed)`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		serveProvider = args[0]

		if serveProvider != "lt" && serveProvider != "baidu" {
			return fmt.Errorf("provider %q not supported for webdav; supported: lt, baidu", serveProvider)
		}

		p, err := getProvider(serveProvider)
		if err != nil {
			return err
		}

		var creds map[string]string

		// Prefer inline token if provided; otherwise load from credential store.
		if serveToken != "" {
			creds = map[string]string{
				"access_token": serveToken,
			}
			if serveUser != "" {
				creds["phone"] = serveUser
			}
		} else {
			creds, err = credential.Load(serveProvider)
			if err != nil {
				return fmt.Errorf("not logged in. Run 'camel login %s' first, or use --token", serveProvider)
			}
		}

		if err := p.Init(nil, creds); err != nil {
			return fmt.Errorf("failed to initialize provider: %w", err)
		}

		// Build auth user/pass for WebDAV Basic auth.
		var authUser, authPass string
		if serveUser != "" {
			authUser = serveUser
			authPass = serveToken // token acts as password
		}

		h := webdav.New(p, authUser, authPass)
		server := &http.Server{
			Addr:    serveAddr,
			Handler: h,
		}

		fmt.Printf("WebDAV server starting on %s (%s)\n", serveAddr, strings.ToUpper(serveProvider))
		fmt.Println("Press Ctrl+C to stop")

		var listenErr error
		if serveTLSCert != "" && serveTLSKey != "" {
			listenErr = server.ListenAndServeTLS(serveTLSCert, serveTLSKey)
		} else {
			listenErr = server.ListenAndServe()
		}

		if listenErr != nil && listenErr != http.ErrServerClosed {
			return fmt.Errorf("server failed: %w", listenErr)
		}

		return nil
	},
}

func init() {
	// Use liantong.New internally (already used by root.go).
	_ = liantong.New

	serveCmd.Flags().StringVarP(&serveAddr, "addr", "a", ":8080", "Listen address (e.g. :8080)")
	serveCmd.Flags().StringVarP(&serveUser, "user", "u", "", "Username for Basic auth")
	serveCmd.Flags().StringVarP(&serveToken, "token", "t", "", "Access token (overrides login credentials)")
	serveCmd.Flags().StringVar(&serveTLSCert, "tls-cert", "", "TLS certificate file path")
	serveCmd.Flags().StringVar(&serveTLSKey, "tls-key", "", "TLS private key file path")
}
