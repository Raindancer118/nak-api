package auth

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/Raindancer118/cis-api/internal/client"
	"github.com/Raindancer118/cis-api/internal/scraper"
)

// Login performs the TYPO3 felogin flow:
//  1. GET / to obtain the login form with hidden TYPO3 fields
//  2. POST credentials + hidden fields to the form action URL
func Login(c *client.Client, username, password string) error {
	resp, err := c.Get("/")
	if err != nil {
		return fmt.Errorf("fetch login page: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read login page: %w", err)
	}

	form, err := scraper.ExtractForm(strings.NewReader(string(body)), "tx_felogin_login")
	if err != nil {
		return fmt.Errorf("parse login form: %w", err)
	}
	if form == nil {
		// Already logged in (no form visible) — treat as success
		if c.IsLoggedIn() {
			return nil
		}
		return fmt.Errorf("login form not found — page layout may have changed")
	}

	// Resolve action URL (may be relative)
	actionURL := form.Action
	if !strings.HasPrefix(actionURL, "http") {
		actionURL = client.BaseURL + actionURL
	}

	// Build form data from hidden fields + credentials
	data := url.Values{}
	for k, v := range form.Fields {
		data.Set(k, v)
	}
	data.Set("user", username)
	data.Set("pass", password)
	data.Set("logintype", "login")
	if _, ok := data["submit"]; !ok {
		data.Set("submit", "Anmelden")
	}

	postResp, err := c.PostFormURL(actionURL, data)
	if err != nil {
		return fmt.Errorf("post login: %w", err)
	}
	defer postResp.Body.Close()
	io.Copy(io.Discard, postResp.Body)

	if !c.IsLoggedIn() {
		return fmt.Errorf("login failed — wrong credentials?")
	}
	return c.SaveSession()
}

// Logout clears the server session and the local cookie store.
func Logout(c *client.Client) error {
	resp, err := c.Get("/login/?logintype=logout")
	if err != nil {
		return fmt.Errorf("logout request: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	c.ClearSession()
	return nil
}
