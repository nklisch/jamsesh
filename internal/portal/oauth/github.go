package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// githubAuthorizeBase and related URL constants are exported as vars so
// tests can substitute a httptest.Server URL via GitHubOptions.BaseURL.
const (
	githubAuthorizeURL   = "https://github.com/login/oauth/authorize"
	githubAccessTokenURL = "https://github.com/login/oauth/access_token"
	githubAPIBase        = "https://api.github.com"

	// githubOAuthHTTPTimeout is the default timeout for outbound OAuth HTTP
	// calls (token exchange, /user, /user/emails). 15s is generous enough for
	// slow networks yet tight enough to prevent goroutine pileup during a
	// GitHub outage. Override via GitHubOptions.HTTPClient for callers with
	// different requirements.
	githubOAuthHTTPTimeout = 15 * time.Second
)

// GitHubOptions configures the GitHub provider. The only required fields are
// ClientID and ClientSecret. Tests inject a custom HTTPClient and BaseURL so
// that httptest.Server can intercept all outbound requests.
type GitHubOptions struct {
	ClientID     string
	ClientSecret string

	// HTTPClient overrides the HTTP client used for GitHub API calls.
	// Defaults to a client with githubOAuthHTTPTimeout when nil.
	HTTPClient *http.Client

	// BaseURL overrides the GitHub API and token-exchange base URL for
	// testing. When set, the provider substitutes it for
	// "https://github.com" (token exchange) and "https://api.github.com"
	// (user profile). Leave empty for production.
	BaseURL string
}

// GitHub implements Provider for GitHub OAuth.
type GitHub struct {
	opts GitHubOptions
}

// NewGitHub constructs a GitHub provider. opts.ClientID and opts.ClientSecret
// must be non-empty; the caller is responsible for validating config before
// calling this.
func NewGitHub(opts GitHubOptions) *GitHub {
	return &GitHub{opts: opts}
}

func (g *GitHub) Name() string { return "github" }

// AuthorizeURL returns the GitHub OAuth authorization URL with the required
// query parameters embedded.
func (g *GitHub) AuthorizeURL(state, redirectURI string) string {
	base := githubAuthorizeURL
	if g.opts.BaseURL != "" {
		base = g.opts.BaseURL + "/login/oauth/authorize"
	}
	q := url.Values{}
	q.Set("client_id", g.opts.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	q.Set("scope", "read:user user:email")
	return base + "?" + q.Encode()
}

// Exchange performs the GitHub OAuth token exchange and returns a normalised
// Identity. It:
//  1. POSTs to /login/oauth/access_token with the code.
//  2. GETs /user for the profile (id, login, name).
//  3. GETs /user/emails and selects the primary+verified entry.
func (g *GitHub) Exchange(ctx context.Context, code, redirectURI string) (Identity, error) {
	accessToken, err := g.exchangeCode(ctx, code, redirectURI)
	if err != nil {
		return Identity{}, &ErrExchange{Provider: "github", Cause: err}
	}

	user, err := g.fetchUser(ctx, accessToken)
	if err != nil {
		return Identity{}, &ErrExchange{Provider: "github", Cause: err}
	}

	email, err := g.fetchPrimaryEmail(ctx, accessToken)
	if err != nil {
		return Identity{}, &ErrExchange{Provider: "github", Cause: err}
	}

	displayName := user.Name
	if displayName == "" {
		displayName = user.Login
	}

	return Identity{
		Provider:    "github",
		ProviderID:  fmt.Sprintf("%d", user.ID),
		Email:       email,
		DisplayName: displayName,
	}, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (g *GitHub) httpClient() *http.Client {
	if g.opts.HTTPClient != nil {
		return g.opts.HTTPClient
	}
	return &http.Client{Timeout: githubOAuthHTTPTimeout}
}

func (g *GitHub) tokenBase() string {
	if g.opts.BaseURL != "" {
		return g.opts.BaseURL
	}
	return "https://github.com"
}

func (g *GitHub) apiBase() string {
	if g.opts.BaseURL != "" {
		return g.opts.BaseURL
	}
	return githubAPIBase
}

// exchangeCode POSTs to GitHub's token endpoint and returns the access token.
func (g *GitHub) exchangeCode(ctx context.Context, code, redirectURI string) (string, error) {
	form := url.Values{}
	form.Set("client_id", g.opts.ClientID)
	form.Set("client_secret", g.opts.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		g.tokenBase()+"/login/oauth/access_token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := g.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, body)
	}

	var tok struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tok.Error != "" {
		return "", fmt.Errorf("github error %s: %s", tok.Error, tok.ErrorDesc)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("github returned empty access_token")
	}
	return tok.AccessToken, nil
}

// githubUser is the subset of /user fields we use.
type githubUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
	Email string `json:"email"` // may be nil/empty; prefer /user/emails
}

func (g *GitHub) fetchUser(ctx context.Context, accessToken string) (githubUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		g.apiBase()+"/user", nil)
	if err != nil {
		return githubUser{}, fmt.Errorf("build user request: %w", err)
	}
	req.Header.Set("Authorization", "token "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := g.httpClient().Do(req)
	if err != nil {
		return githubUser{}, fmt.Errorf("user request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return githubUser{}, fmt.Errorf("/user returned %d: %s", resp.StatusCode, body)
	}

	var u githubUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return githubUser{}, fmt.Errorf("decode user response: %w", err)
	}
	return u, nil
}

// githubEmail is one entry from the /user/emails array.
type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

// fetchPrimaryEmail fetches /user/emails and returns the primary+verified
// email address. Falls back to the first entry if no primary+verified one
// is found (defensive — GitHub always provides one).
func (g *GitHub) fetchPrimaryEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		g.apiBase()+"/user/emails", nil)
	if err != nil {
		return "", fmt.Errorf("build emails request: %w", err)
	}
	req.Header.Set("Authorization", "token "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := g.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("emails request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("/user/emails returned %d: %s", resp.StatusCode, body)
	}

	var emails []githubEmail
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", fmt.Errorf("decode emails response: %w", err)
	}

	// Primary + verified is the canonical pick.
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	// Fall back to first primary (unverified) or just the first entry.
	for _, e := range emails {
		if e.Primary {
			return e.Email, nil
		}
	}
	if len(emails) > 0 {
		return emails[0].Email, nil
	}
	return "", fmt.Errorf("github returned no email addresses")
}
