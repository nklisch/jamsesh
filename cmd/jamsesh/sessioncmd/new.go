package sessioncmd

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/mattn/go-isatty"
	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/portalclient"
	"jamsesh/cmd/jamsesh/state"
	"jamsesh/internal/api/openapi"
)

// isTTY is a package-level variable so tests can override it without a real terminal.
var isTTY = func(f *os.File) bool {
	return isatty.IsTerminal(f.Fd())
}

// runGitWithEnv runs git with additional args (including -c flags) and an optional
// environment overlay. It inherits stdout/stderr so git's progress output is visible.
// Override in tests.
var runGitWithEnv = func(env []string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	return cmd.Run()
}

// NewCommand returns the urfave/cli command descriptor for "jamsesh new".
func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "new",
		Usage: "Create a session from the current repo checkout",
		Description: "Creates a session on the portal, pushes local HEAD as base ref, " +
			"and writes per-session state for subsequent jamsesh invocations. " +
			"Run from inside a git checkout. " +
			"In a Claude Code agent session the agent should pass --org and other " +
			"flags explicitly; interactive prompts are for direct human use only.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "org", Usage: "Org ID (required when stdin is not a TTY)"},
			&cli.StringFlag{Name: "name", Usage: "Session name (default: jam-<timestamp>)"},
			&cli.StringFlag{Name: "goal", Usage: "Session goal"},
			&cli.StringFlag{Name: "scope", Value: "**", Usage: "Writable scope as a single glob or JSON array (default: '**')"},
			&cli.StringFlag{Name: "mode", Value: "sync", Usage: "Default mode (sync|isolated)"},
			&cli.StringFlag{Name: "invite", Usage: "Comma-separated emails to invite after creation"},
			&cli.BoolFlag{Name: "non-interactive", Usage: "Skip all prompts; require all params via flags"},
			&cli.BoolFlag{Name: "playground", Usage: "Create an ephemeral anonymous playground session (no auth required)"},
		},
		Action: newAction,
	}
}

// CreateParams holds the resolved parameters for creating a session.
type CreateParams struct {
	OrgID       string
	Name        string // never empty by the time resolveCreateParams returns
	Goal        string // may be empty
	Scope       string // never empty; JSON array of globs
	DefaultMode string // "sync" or "isolated"
}

// newAction is the urfave/cli action for "jamsesh new".
func newAction(ctx context.Context, cmd *cli.Command) error {
	// Mutual-exclusion guard: --playground and --org are incompatible.
	if cmd.Bool("playground") && cmd.String("org") != "" {
		return errors.New("--playground and --org are mutually exclusive: playground sessions are always in the reserved playground org")
	}

	// Playground path: skip auth, org picker, and durable-session creation.
	if cmd.Bool("playground") {
		return newPlaygroundAction(ctx, cmd)
	}

	// 1. Construct portal client (reads portal URL + token via state helpers)
	pc, err := buildPortalClient()
	if err != nil {
		return err
	}

	// 2. Resolve all params (flags + prompts + defaults)
	params, err := resolveCreateParams(ctx, cmd, pc)
	if err != nil {
		return err
	}

	// 3. Call portal API to create session row + member row
	session, err := createSessionAPI(ctx, pc, params)
	if err != nil {
		return err
	}

	// 4. Push local HEAD as base ref (may fail; if so, leave session live)
	pushErr := pushBaseRef(ctx, pc, session.Id)
	if pushErr != nil {
		// Per locked decision: session stays live with base_sha NULL.
		// CLI prints retry command, returns wrapped error.
		return wrapPushError(pushErr, session, pc.BaseURL)
	}

	// 5. Write per-session state files for subsequent jamsesh invocations
	if err := writeNewSessionState(session, params); err != nil {
		return err
	}

	// 6. If --invite flag set, send invites (best-effort; reports failures
	//    but doesn't fail the whole create)
	if invites := strings.TrimSpace(cmd.String("invite")); invites != "" {
		emails := parseInviteEmails(invites)
		if err := sendInvitesIfRequested(ctx, pc, session.OrgId, session.Id, emails); err != nil {
			// Print warning but don't fail; session is live and pushed.
			fmt.Fprintf(os.Stderr, "warning: invites partially failed: %v\n", err)
		}
	}

	// 7. Print success summary
	printSuccessSummary(session, params, pc.BaseURL)

	// 8. Update most-recently-used org for next time's prompt pre-selection (best-effort)
	_ = state.Write("last_org_id", []byte(params.OrgID), 0o600)

	return nil
}

// resolveCreateParams resolves all session creation parameters from flags,
// interactive prompts, and defaults.
func resolveCreateParams(ctx context.Context, cmd *cli.Command, pc *portalclient.Client) (CreateParams, error) {
	nonInteractive := cmd.Bool("non-interactive") || !isTTY(os.Stdin)

	params := CreateParams{
		OrgID:       cmd.String("org"),
		Name:        cmd.String("name"),
		Goal:        cmd.String("goal"),
		Scope:       cmd.String("scope"),
		DefaultMode: cmd.String("mode"),
	}

	// Org: required when non-interactive; picker when interactive multi-org
	if params.OrgID == "" {
		if nonInteractive {
			return CreateParams{}, errors.New(
				"non-interactive mode: pass `--org <id>` to specify the org " +
					"(use `jamsesh status --json` to list your orgs)")
		}
		picked, err := pickOrgInteractive(ctx, pc)
		if err != nil {
			return CreateParams{}, err
		}
		params.OrgID = picked
	}

	// Name: auto-generate if blank (jam-<unix-timestamp>)
	if params.Name == "" {
		params.Name = fmt.Sprintf("jam-%d", time.Now().Unix())
	}

	// Scope: normalize to JSON array (accept single glob or JSON array)
	normalized, err := normalizeScope(params.Scope)
	if err != nil {
		return CreateParams{}, fmt.Errorf("invalid --scope: %w", err)
	}
	params.Scope = normalized

	// Mode: validate
	if params.DefaultMode != "sync" && params.DefaultMode != "isolated" {
		return CreateParams{}, fmt.Errorf("invalid --mode %q: must be sync or isolated", params.DefaultMode)
	}

	return params, nil
}

// pickOrgInteractive fetches the user's org memberships and presents a
// numbered-list picker on stdin. Returns the chosen org ID.
func pickOrgInteractive(ctx context.Context, pc *portalclient.Client) (string, error) {
	me, err := portalclient.GetJSON[openapi.MeResponse](ctx, pc, "/api/me")
	if err != nil {
		return "", fmt.Errorf("fetch /api/me: %w", err)
	}

	if len(me.Orgs) == 0 {
		return "", errors.New("no org memberships yet; create one in the portal UI first")
	}
	if len(me.Orgs) == 1 {
		return me.Orgs[0].Id, nil // no picker needed
	}

	// Pre-select most-recently-used (best-effort read)
	preselected, _ := state.Read("last_org_id")

	// Numbered-list picker on stdin (simple; matches the existing auth flow's
	// stdin-bufio.Scanner idiom; no arrow-key TUI dependency)
	fmt.Println("Which org for this session?")
	defaultIdx := 0
	for i, org := range me.Orgs {
		marker := " "
		if string(preselected) == org.Id {
			marker = "*"
			defaultIdx = i
		}
		fmt.Printf("  [%d]%s %s (%s)\n", i+1, marker, org.Name, org.Id)
	}
	fmt.Printf("Pick a number [1-%d, default %d]: ", len(me.Orgs), defaultIdx+1)

	line, err := readStdinLine()
	if err != nil {
		return "", fmt.Errorf("read picker input: %w", err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return me.Orgs[defaultIdx].Id, nil
	}
	pick, err := strconv.Atoi(line)
	if err != nil || pick < 1 || pick > len(me.Orgs) {
		return "", fmt.Errorf("invalid pick %q: must be a number between 1 and %d", line, len(me.Orgs))
	}
	return me.Orgs[pick-1].Id, nil
}

// readStdinLine is a package-level variable so tests can override stdin reading.
var readStdinLine = func() (string, error) {
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return scanner.Text(), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", nil // EOF → empty line → default pick
}

// createSessionAPI calls the portal API to create a session row and returns
// the created Session.
func createSessionAPI(ctx context.Context, pc *portalclient.Client, p CreateParams) (openapi.Session, error) {
	req := openapi.CreateSessionRequest{
		Name:        p.Name,
		Goal:        p.Goal, // may be empty string; portal accepts "" (validates max 4096 chars)
		Scope:       p.Scope,
		DefaultMode: openapi.CreateSessionRequestDefaultMode(p.DefaultMode),
	}
	path := fmt.Sprintf("/api/orgs/%s/sessions", url.PathEscape(p.OrgID))
	return portalclient.PostJSON[openapi.Session](ctx, pc, path, req)
}

// pushBaseRef pushes the local HEAD to refs/heads/jam/<sessionID>/base on the
// portal-side bare repo. Credentials are injected via -c http.extraHeader
// (NOT URL-embedded) to prevent token leakage into git's reflog or `git remote -v`.
// No `git remote add` is performed — the push is transient, leaving .git/config clean.
func pushBaseRef(ctx context.Context, pc *portalclient.Client, sessionID string) error {
	// Verify we're in a git checkout with a HEAD
	if err := runGit("rev-parse", "--git-dir"); err != nil {
		return fmt.Errorf("not a git checkout: %w", err)
	}
	headSHA, err := runGitOutput("rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("repo has no commits yet (nothing to push as base): %w", err)
	}
	headSHA = strings.TrimSpace(headSHA)
	_ = headSHA // used for validation; actual push uses HEAD refspec

	// Read the current token for Basic auth injection.
	// We use -c http.extraHeader rather than embedding the token in the URL because:
	//   1. URL-embedded credentials appear in git's process listing (visible to other
	//      users on shared systems via `ps aux`).
	//   2. They can appear in git's reflog and `git remote -v` output.
	//   3. Header injection is process-local and does not persist anywhere.
	// sessionID is the just-created session; ReadCurrentBearer checks the per-session
	// token store first and falls back to the legacy account-wide file if absent.
	token, err := state.ReadCurrentBearer(sessionID)
	if err != nil {
		return fmt.Errorf("read token for push: %w", err)
	}

	remoteURL := strings.TrimRight(pc.BaseURL, "/") + "/git/" + sessionID + ".git"
	// HTTP Basic: username is arbitrary ("jamsesh"), password is the OAuth bearer token.
	basicHeader := "Authorization: Basic " + base64.StdEncoding.EncodeToString(
		[]byte("jamsesh:"+token))

	// Refspec: push HEAD to refs/heads/jam/<sessionID>/base on the remote.
	// The pre-receive hook on the portal validates this ref and stamps base_sha.
	refspec := "HEAD:refs/heads/jam/" + sessionID + "/base"
	return runGitWithEnv(
		[]string{}, // no extra env vars beyond the system environment
		"-c", "http.extraHeader="+basicHeader,
		"push", remoteURL, refspec,
	)
}

// writeNewSessionState writes the per-session state files under
// ${CLAUDE_PLUGIN_DATA}/sessions/<sessionID>/. The instance_id binding is NOT
// written here because jamsesh new may be run from plain bash without a CC
// instance attached — binding happens at first attach (via /jamsesh:join or
// equivalent).
func writeNewSessionState(session openapi.Session, params CreateParams) error {
	dir, err := state.PluginDataDir()
	if err != nil {
		return err
	}

	sessDir := filepath.Join(dir, "sessions", session.Id)
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		return fmt.Errorf("creating session state dir: %w", err)
	}

	// Find the creator's account ID — the creator is the only member at this point.
	creatorAccountID := ""
	if len(session.Members) > 0 {
		creatorAccountID = session.Members[0].AccountId
	}

	ref := fmt.Sprintf("jam/%s/%s/main", session.Id, creatorAccountID)

	writes := []struct{ name, value string }{
		{"sessions/" + session.Id + "/ref", ref},
		{"sessions/" + session.Id + "/org_id", params.OrgID},
		{"sessions/" + session.Id + "/account_id", creatorAccountID},
		{"sessions/" + session.Id + "/last_seen_seq", "0"},
	}
	for _, w := range writes {
		if err := state.Write(w.name, []byte(w.value), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", w.name, err)
		}
	}
	// instance_id is intentionally NOT written here; see function doc.
	return nil
}

// buildPortalClient constructs a portal client from local state, wiring token
// refresh. Extracted for testability.
func buildPortalClient() (*portalclient.Client, error) {
	portalURL, err := state.ReadPortalURL()
	if err != nil {
		return nil, fmt.Errorf("resolving portal URL: %w", err)
	}
	// Verify a token is available early so we get a useful error before touching the network.
	// Pre-binding: no session ID yet, pass "" so ReadCurrentBearer uses the legacy path.
	if _, err := state.ReadCurrentBearer(""); err != nil {
		return nil, fmt.Errorf("not authenticated — run `jamsesh auth` first: %w", err)
	}
	pc := &portalclient.Client{BaseURL: portalURL}
	portalclient.WireRefresh(pc)
	return pc, nil
}

// buildPlaygroundClient returns the base portal URL for unauthenticated
// playground requests. Unlike buildPortalClient it does NOT check for a
// stored token — the playground endpoint is public (no auth required).
// The HTTP client (nil = http.DefaultClient) is returned separately so the
// caller can pass it directly to PostJSONAnon / pushBaseRefWithBearer.
func buildPlaygroundClient() (string, *http.Client, error) {
	portalURL, err := state.ReadPortalURL()
	if err != nil {
		return "", nil, fmt.Errorf("resolving portal URL: %w", err)
	}
	return portalURL, nil, nil
}

// newPlaygroundAction handles `jamsesh new --playground`. It creates an
// ephemeral anonymous playground session (no auth required), pushes the
// local HEAD as base ref using the just-received bearer, writes per-session
// state, and prints a playground-specific success summary.
func newPlaygroundAction(ctx context.Context, cmd *cli.Command) error {
	baseURL, hc, err := buildPlaygroundClient()
	if err != nil {
		return err
	}

	// Build the create request. All fields optional; server supplies defaults.
	req := openapi.CreatePlaygroundSessionRequest{
		Name:  cmd.String("name"),
		Goal:  cmd.String("goal"),
		Scope: cmd.String("scope"),
	}
	// scope defaults to "**" via the flag Default; normalise it.
	// The default "**" must be normalised the same as any user-supplied value
	// because the portal validates scope as a JSON array via
	// prereceive.ValidateWritableScope — a raw "**" string is rejected with
	// session.invalid_writable_scope.
	if req.Scope != "" {
		normalized, err := normalizeScope(req.Scope)
		if err != nil {
			return fmt.Errorf("invalid --scope: %w", err)
		}
		req.Scope = normalized
	}

	resp, err := portalclient.PostJSONAnon[openapi.PlaygroundSessionCreated](
		ctx, hc, baseURL, "/api/playground/sessions", req)
	if err != nil {
		return fmt.Errorf("creating playground session: %w", err)
	}

	// Persist the just-received bearer to per-session token storage immediately.
	if err := state.WriteSessionToken(resp.Session.Id, []byte(resp.Bearer)); err != nil {
		return fmt.Errorf("writing playground session token: %w", err)
	}

	// Push local HEAD as base ref using the just-received bearer (no OAuth token).
	if err := pushBaseRefWithBearer(ctx, baseURL, resp.Session.Id, resp.Bearer); err != nil {
		// Session stays live with base_sha NULL per locked decision.
		return wrapPlaygroundPushError(err, resp.Session, baseURL)
	}

	// Write per-session state files.
	if err := writePlaygroundSessionState(resp.Session); err != nil {
		return err
	}

	printPlaygroundSummary(resp, baseURL)
	return nil
}

// pushBaseRefWithBearer is a variant of pushBaseRef that uses an explicit
// bearer token instead of reading the account-wide OAuth token from state.
// This is used for playground sessions where the user may not have (or need)
// an OAuth token — the anonymous session bearer is sufficient.
// Credentials are injected via -c http.extraHeader (NOT URL-embedded) to
// prevent token leakage into git's reflog or `git remote -v` output.
func pushBaseRefWithBearer(ctx context.Context, baseURL, sessionID, bearer string) error {
	_ = ctx // reserved for future cancellation propagation

	// Verify we're in a git checkout with a HEAD.
	if err := runGit("rev-parse", "--git-dir"); err != nil {
		return fmt.Errorf("not a git checkout: %w", err)
	}
	headSHA, err := runGitOutput("rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("repo has no commits yet (nothing to push as base): %w", err)
	}
	headSHA = strings.TrimSpace(headSHA)
	_ = headSHA // validated; actual push uses HEAD refspec

	// The portal's git smart-HTTP route is /git/{orgID}/{sessionID}.git/...
	// (see internal/portal/githttp/handler.go:90). Playground sessions live
	// under the reserved "org_playground" org.
	remoteURL := strings.TrimRight(baseURL, "/") + "/git/org_playground/" + sessionID + ".git"
	// HTTP Basic: username is arbitrary ("jamsesh"), password is the bearer token.
	basicHeader := "Authorization: Basic " + base64.StdEncoding.EncodeToString(
		[]byte("jamsesh:"+bearer))

	refspec := "HEAD:refs/heads/jam/" + sessionID + "/base"
	return runGitWithEnv(
		[]string{},
		"-c", "http.extraHeader="+basicHeader,
		"push", remoteURL, refspec,
	)
}

// writePlaygroundSessionState writes per-session state files for a playground
// session. The org_id is always "org_playground" and there is no account_id
// to bind at create time (no durable account — the anonymous bearer is the
// credential).
func writePlaygroundSessionState(session openapi.PlaygroundSessionSummary) error {
	dir, err := state.PluginDataDir()
	if err != nil {
		return err
	}

	sessDir := filepath.Join(dir, "sessions", session.Id)
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		return fmt.Errorf("creating session state dir: %w", err)
	}

	// For playground sessions the ref is jam/<sessionID>/playground/main;
	// there is no per-account ref variant because the account is anonymous.
	ref := fmt.Sprintf("jam/%s/playground/main", session.Id)

	writes := []struct{ name, value string }{
		{"sessions/" + session.Id + "/ref", ref},
		{"sessions/" + session.Id + "/org_id", session.OrgId},
		{"sessions/" + session.Id + "/last_seen_seq", "0"},
	}
	for _, w := range writes {
		if err := state.Write(w.name, []byte(w.value), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", w.name, err)
		}
	}
	return nil
}

// wrapPlaygroundPushError wraps a push failure for the playground path,
// including the explicit retry command. Session stays live per locked decision.
func wrapPlaygroundPushError(pushErr error, session openapi.PlaygroundSessionSummary, baseURL string) error {
	remoteURL := strings.TrimRight(baseURL, "/") + "/git/org_playground/" + session.Id + ".git"
	retryCmd := fmt.Sprintf("git push %s HEAD:refs/heads/jam/%s/base", remoteURL, session.Id)
	return fmt.Errorf(
		"push failed (playground session %s is live with base_sha: null): %w\n"+
			"Retry with:\n  %s",
		session.Id, pushErr, retryCmd)
}

// printPlaygroundSummary prints a human-readable summary for a newly created
// playground session, highlighting the share URL, nickname, and expiry.
func printPlaygroundSummary(resp openapi.PlaygroundSessionCreated, baseURL string) {
	shareURL := strings.TrimRight(baseURL, "/") + "/playground/" + resp.Session.Id
	expiresIn := time.Until(resp.ExpiresAt).Round(time.Minute)

	fmt.Printf("Playground session created!\n")
	fmt.Printf("  Share URL:  %s\n", shareURL)
	fmt.Printf("  You are:    %s\n", resp.Nickname)
	fmt.Printf("  Session:    %s (%s)\n", resp.Session.Name, resp.Session.Id)
	if resp.Session.Goal != "" {
		fmt.Printf("  Goal:       %s\n", resp.Session.Goal)
	}
	fmt.Printf("  Ends:       in %s (hard cap) or after idle timeout\n", expiresIn)
	fmt.Printf("Base ref pushed. Others can join at:\n  %s\n", shareURL)
}

// wrapPushError wraps a push failure with context including the explicit retry
// command. The session is left live (not abandoned) per the locked design decision.
func wrapPushError(pushErr error, session openapi.Session, baseURL string) error {
	// Pre-existing bug: this retry hint URL is missing the org_id segment that
	// the portal's git route actually expects (/git/{orgID}/{sessionID}.git).
	// See bug-playground-git-receive-pack-fails-with-200-hangup.md — the
	// durable-session variant of the same wiring issue. Not fixed here to keep
	// the playground autopilot scope narrow; track separately.
	remoteURL := strings.TrimRight(baseURL, "/") + "/git/" + session.Id + ".git"
	retryCmd := fmt.Sprintf("git push %s HEAD:refs/heads/jam/%s/base", remoteURL, session.Id)
	return fmt.Errorf(
		"push failed (session %s is live with base_sha: null): %w\n"+
			"Retry with:\n  %s",
		session.Id, pushErr, retryCmd)
}

// printSuccessSummary prints a human-readable summary of the created session.
func printSuccessSummary(session openapi.Session, params CreateParams, baseURL string) {
	sessionURL := strings.TrimRight(baseURL, "/") + "/orgs/" + session.OrgId + "/sessions/" + session.Id
	fmt.Printf("Session created: %s (%s)\n", session.Name, session.Id)
	fmt.Printf("  URL:    %s\n", sessionURL)
	fmt.Printf("  Org:    %s\n", params.OrgID)
	if session.Goal != "" {
		fmt.Printf("  Goal:   %s\n", session.Goal)
	}
	fmt.Printf("  Scope:  %s\n", session.Scope)
	fmt.Printf("  Mode:   %s\n", session.DefaultMode)
	fmt.Printf("Base ref pushed. To join this session:\n  jamsesh join %s\n", session.Id)
}

// normalizeScope accepts either a single glob string (e.g. "docs/**") or a
// JSON array string (e.g. '["docs/**","src/*.go"]') and always returns a JSON
// array string. Single globs are validated via doublestar.
func normalizeScope(s string) (string, error) {
	s = strings.TrimSpace(s)
	// Already a JSON array?
	if strings.HasPrefix(s, "[") {
		// Validate it parses as a JSON array of strings.
		var globs []string
		if err := json.Unmarshal([]byte(s), &globs); err != nil {
			return "", fmt.Errorf("scope %q is not a valid JSON array of strings: %w", s, err)
		}
		// Re-marshal to normalize whitespace.
		out, err := json.Marshal(globs)
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
	// Single glob — validate and wrap.
	if _, err := doublestar.Match(s, ""); err != nil {
		return "", fmt.Errorf("invalid glob pattern %q: %w", s, err)
	}
	out, err := json.Marshal([]string{s})
	if err != nil {
		return "", err
	}
	return string(out), nil
}
