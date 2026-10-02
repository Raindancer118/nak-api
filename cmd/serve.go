package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Raindancer118/nak-api/internal/app"
	"github.com/Raindancer118/nak-api/internal/tools"
	"github.com/Raindancer118/nak-api/internal/web"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web UI (single user, token login)",
	Long: `Start the web UI and its JSON API.

  --addr / NAK_WEB_ADDR          listen address (default 127.0.0.1:8080)
  NAK_WEB_TOKEN                  access token; otherwise one is generated and
                                 kept in the data dir (file web-token)
  --cache-ttl / NAK_WEB_CACHE_TTL how long read results are reused (default 5m)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		addr, _ := cmd.Flags().GetString("addr")
		ttl, _ := cmd.Flags().GetDuration("cache-ttl")
		a := app.FromEnv()

		token := strings.TrimSpace(os.Getenv("NAK_WEB_TOKEN"))
		fromFile := token == ""
		created := false
		if fromFile {
			var err error
			if token, created, err = web.LoadOrCreateToken(a.ConfigDir); err != nil {
				return fmt.Errorf("access token: %w", err)
			}
		}

		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}
		srv := &http.Server{
			Handler:           web.New(a, tools.All(), web.Config{Token: token, CacheTTL: ttl, Version: Version}).Handler(),
			ReadHeaderTimeout: 10 * time.Second,
			// tool calls (nak_dashboard, downloads) can take a while
			WriteTimeout: 5 * time.Minute,
			IdleTimeout:  2 * time.Minute,
		}
		base := "http://" + displayAddr(ln.Addr().String())
		log.Printf("nak %s web UI on %s", Version, base)
		switch {
		case created:
			// shown once; the #fragment never reaches a server or proxy log
			log.Printf("new access token stored in %s/web-token", a.ConfigDir)
			log.Printf("log in: %s/login#token=%s", base, token)
		case fromFile:
			log.Printf("access token: see %s/web-token", a.ConfigDir)
		}
		if a.ReadOnly {
			log.Printf("read-only mode: binding actions are blocked")
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		errc := make(chan error, 1)
		go func() { errc <- srv.Serve(ln) }()
		select {
		case err := <-errc:
			return err
		case <-ctx.Done():
		}
		log.Printf("shutting down")
		sctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := srv.Shutdown(sctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	},
}

// displayAddr turns a wildcard listen address into one a browser can open.
func displayAddr(a string) string {
	host, port, err := net.SplitHostPort(a)
	if err != nil {
		return a
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "localhost"
	}
	return net.JoinHostPort(host, port)
}

var healthcheckCmd = &cobra.Command{
	Use:   "healthcheck",
	Short: "Exit 0 if the web UI answers on /healthz (for container health checks)",
	RunE: func(cmd *cobra.Command, args []string) error {
		u, _ := cmd.Flags().GetString("url")
		if u == "" {
			_, port, err := net.SplitHostPort(envOrDefault("NAK_WEB_ADDR", "127.0.0.1:8080"))
			if err != nil {
				return err
			}
			u = "http://127.0.0.1:" + port + "/healthz"
		}
		c := &http.Client{Timeout: 5 * time.Second}
		res, err := c.Get(u)
		if err != nil {
			return err
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("%s: HTTP %d", u, res.StatusCode)
		}
		return nil
	},
}

func envOrDefault(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}

func init() {
	ttl := 5 * time.Minute
	if v := os.Getenv("NAK_WEB_CACHE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			ttl = d
		}
	}
	serveCmd.Flags().String("addr", envOrDefault("NAK_WEB_ADDR", "127.0.0.1:8080"), "listen address")
	serveCmd.Flags().Duration("cache-ttl", ttl, "reuse read results for this long")
	healthcheckCmd.Flags().String("url", "", "health URL (default from NAK_WEB_ADDR)")
	rootCmd.AddCommand(serveCmd, healthcheckCmd)
}
