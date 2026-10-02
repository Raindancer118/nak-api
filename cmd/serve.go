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
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Raindancer118/nak-api/internal/app"
	"github.com/Raindancer118/nak-api/internal/demo"
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
  --cache-ttl / NAK_WEB_CACHE_TTL one freshness for all tools (default: per tool,
                                 1 min for messages up to 2 h for grades)
  NAK_WEB_UI_DIR                 serve the UI from this directory (UI development)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		addr, _ := cmd.Flags().GetString("addr")
		ttl, _ := cmd.Flags().GetDuration("cache-ttl")
		isDemo, _ := cmd.Flags().GetBool("demo")
		a := app.FromEnv()
		reg := tools.All()
		if isDemo {
			// made-up data, nothing leaves the machine; keep it apart from real data
			reg = demo.Registry()
			dir, err := os.MkdirTemp("", "naknak-demo-")
			if err != nil {
				return err
			}
			a.ConfigDir, a.DownloadDir = dir, filepath.Join(dir, "downloads")
			defer os.RemoveAll(dir)
			log.Printf("DEMO mode: invented data, no NAK account, nothing is sent anywhere")
		}

		token := strings.TrimSpace(os.Getenv("NAK_WEB_TOKEN"))
		if isDemo && token == "" {
			token = "demo" // the demo needs no login; nothing to protect
		}
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
		ws := web.New(a, reg, web.Config{Token: token, CacheTTL: ttl, Version: Version,
			UIDir: os.Getenv("NAK_WEB_UI_DIR"), CacheFile: filepath.Join(a.ConfigDir, "web-cache.json"), Demo: isDemo})
		srv := &http.Server{
			Handler:           ws.Handler(),
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
		ws.StartWatcher(ctx)
		errc := make(chan error, 1)
		go func() { errc <- srv.Serve(ln) }()
		select {
		case err := <-errc:
			return err
		case <-ctx.Done():
		}
		log.Printf("shutting down")
		// save first: docker stop kills after 10 s
		if serr := ws.SaveCache(); serr != nil {
			log.Printf("saving cache: %v", serr)
		}
		ws.CloseStreams()
		sctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		err = srv.Shutdown(sctx)
		ws.SaveCache() // anything that finished while draining
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
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
	var ttl time.Duration
	if v := os.Getenv("NAK_WEB_CACHE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			ttl = d
		}
	}
	serveCmd.Flags().String("addr", envOrDefault("NAK_WEB_ADDR", "127.0.0.1:8080"), "listen address")
	serveCmd.Flags().Duration("cache-ttl", ttl, "reuse read results for this long")
	serveCmd.Flags().Bool("demo", false, "run with invented data (try naknak without a NAK account)")
	healthcheckCmd.Flags().String("url", "", "health URL (default from NAK_WEB_ADDR)")
	rootCmd.AddCommand(serveCmd, healthcheckCmd)
}
