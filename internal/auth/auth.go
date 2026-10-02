// Package auth performs the TYPO3 felogin flow of the CIS.
package auth

import (
	"fmt"
	"os"

	"github.com/Raindancer118/nak-api/internal/client"
	"github.com/Raindancer118/nak-api/internal/forms"
	"github.com/Raindancer118/nak-api/internal/htmlx"
)

// Login fetches the login form (hidden TYPO3 fields included), posts the
// credentials and stores the session cookie.
func Login(c *client.Client, username, password string) error {
	c.ClearSession()
	p, err := c.GetRaw("/")
	if err != nil {
		return fmt.Errorf("fetch login page: %w", err)
	}
	f := forms.FindByField(forms.Parse(htmlx.MustParse(p.Body), c.Base), "pass")
	if f == nil {
		return fmt.Errorf("login form not found — page layout may have changed")
	}
	set := map[string]string{"user": username, "pass": password}
	if f.Field("logintype") != nil {
		set["logintype"] = "login"
	}
	s, err := f.Submit("", set)
	if err != nil {
		return fmt.Errorf("build login: %w", err)
	}
	res, err := c.PostLogin(s.URL, s.Values())
	if err != nil {
		return fmt.Errorf("post login: %w", err)
	}
	if client.LooksLoggedOut(res) || !c.IsLoggedIn() {
		return fmt.Errorf("login failed — wrong credentials?")
	}
	return c.SaveSession()
}

// Logout ends the server session and removes the local cookie store.
func Logout(c *client.Client) error {
	_, err := c.GetRaw("/login/?logintype=logout")
	c.ClearSession()
	if err != nil {
		return fmt.Errorf("logout request: %w", err)
	}
	return nil
}

// EnvRelogin returns a Relogin hook using CIS_USER/CIS_PASS (nil if unset).
func EnvRelogin() func(*client.Client) error {
	u, p := os.Getenv("CIS_USER"), os.Getenv("CIS_PASS")
	if u == "" || p == "" {
		return nil
	}
	return func(c *client.Client) error { return Login(c, u, p) }
}
