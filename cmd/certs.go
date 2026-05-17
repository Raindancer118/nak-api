package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Raindancer118/cis-api/internal/certs"
	"github.com/Raindancer118/cis-api/internal/client"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
)

var certsCmd = &cobra.Command{
	Use:   "certs",
	Short: "List and download certificates (Online-Bescheinigungen)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.New()
		if err != nil {
			return err
		}
		if !c.IsLoggedIn() {
			return fmt.Errorf("not logged in — run 'cis login' first")
		}

		download, _ := cmd.Flags().GetInt("download")
		outDir, _ := cmd.Flags().GetString("out")

		fmt.Fprintln(os.Stderr, "Fetching certificates...")
		list, err := certs.FetchList(c)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			fmt.Println("No downloadable certificates found.")
			return nil
		}

		if download == 0 {
			// Just list
			t := table.NewWriter()
			t.SetOutputMirror(os.Stdout)
			t.SetStyle(table.StyleLight)
			t.AppendHeader(table.Row{"#", "Name", "URL"})
			for i, cert := range list {
				t.AppendRow(table.Row{i + 1, cert.Name, cert.DownloadURL})
			}
			t.Render()
			fmt.Fprintf(os.Stderr, "\nUse --download <#> to download a certificate.\n")
			return nil
		}

		if download < 1 || download > len(list) {
			return fmt.Errorf("invalid index %d — choose 1..%d", download, len(list))
		}
		cert := list[download-1]
		fmt.Fprintf(os.Stderr, "Downloading: %s\n", cert.Name)
		data, contentType, err := certs.Download(c, cert.DownloadURL)
		if err != nil {
			return err
		}

		ext := extensionFor(contentType)
		filename := cert.Name + ext
		if outDir != "" {
			filename = filepath.Join(outDir, filename)
		}
		if err := os.WriteFile(filename, data, 0600); err != nil {
			return fmt.Errorf("write file: %w", err)
		}
		fmt.Printf("Saved: %s (%d bytes)\n", filename, len(data))
		return nil
	},
}

func extensionFor(contentType string) string {
	switch {
	case strings.Contains(contentType, "pdf"):
		return ".pdf"
	case strings.Contains(contentType, "zip"):
		return ".zip"
	default:
		return ".bin"
	}
}

func init() {
	certsCmd.Flags().IntP("download", "d", 0, "Download certificate by index (see list)")
	certsCmd.Flags().StringP("out", "o", ".", "Output directory for downloads")
}
