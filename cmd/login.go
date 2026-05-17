package cmd

import (
	"fmt"
	"os"
	"syscall"

	"github.com/Raindancer118/cis-api/internal/auth"
	"github.com/Raindancer118/cis-api/internal/client"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to the CIS (credentials stored as session cookie)",
	RunE: func(cmd *cobra.Command, args []string) error {
		username, _ := cmd.Flags().GetString("user")
		if username == "" {
			username = os.Getenv("CIS_USER")
		}
		if username == "" {
			fmt.Print("Username: ")
			fmt.Scan(&username)
		}

		password, _ := cmd.Flags().GetString("pass")
		if password == "" {
			password = os.Getenv("CIS_PASS")
		}
		if password == "" {
			fmt.Print("Password: ")
			raw, err := term.ReadPassword(syscall.Stdin)
			fmt.Println()
			if err != nil {
				return fmt.Errorf("read password: %w", err)
			}
			password = string(raw)
		}

		c, err := client.New()
		if err != nil {
			return err
		}

		fmt.Printf("Logging in as %s ...\n", username)
		if err := auth.Login(c, username, password); err != nil {
			return err
		}
		fmt.Println("Logged in. Session saved to ~/.config/cis-api/session.json")
		return nil
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out and remove the local session",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.New()
		if err != nil {
			return err
		}
		if err := auth.Logout(c); err != nil {
			return err
		}
		fmt.Println("Logged out.")
		return nil
	},
}

func init() {
	loginCmd.Flags().StringP("user", "u", "", "CIS username (or set CIS_USER env var)")
	loginCmd.Flags().StringP("pass", "p", "", "CIS password (or set CIS_PASS env var)")
}
