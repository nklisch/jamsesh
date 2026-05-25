package sessioncmd

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/portalclient"
	"jamsesh/cmd/jamsesh/state"
	"jamsesh/internal/api/openapi"
)

const playgroundOrgID = "org_playground"

// StatusCommand returns the urfave/cli command descriptor for "jamsesh status".
func StatusCommand() *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "Show status for all bound sessions",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output status as JSON instead of human-readable text",
			},
		},
		Action: statusAction,
	}
}

// durableStatusOutput is the per-session entry in the --json "durable" array.
type durableStatusOutput struct {
	SessionID string            `json:"session_id"`
	OrgID     string            `json:"org_id"`
	Name      string            `json:"name"`
	Goal      string            `json:"goal"`
	Mode      string            `json:"mode"`
	YourRef   string            `json:"your_ref"`
	Refs      []openapi.Ref     `json:"refs"`
	Members   []openapi.MemberSummary `json:"members"`
	Comments  []openapi.Comment `json:"unresolved_comments"`
}

// playgroundStatusOutput is the per-session entry in --json "playground" array.
type playgroundStatusOutput struct {
	SessionID  string    `json:"session_id"`
	OrgID      string    `json:"org_id"`
	Name       string    `json:"name"`
	Nickname   string    `json:"nickname"`
	HardCapAt  time.Time `json:"hard_cap_at"`
	IdleTimeout time.Time `json:"idle_timeout_at"`
	Members    int       `json:"members_count"`
	Status     string    `json:"status"`
}

// statusJSONOutput is the top-level --json envelope.
type statusJSONOutput struct {
	Durable    []durableStatusOutput    `json:"durable"`
	Playground []playgroundStatusOutput `json:"playground"`
}

func statusAction(ctx context.Context, cmd *cli.Command) error {
	asJSON := cmd.Bool("json")

	sessions, err := state.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	if len(sessions) == 0 {
		if asJSON {
			return json.NewEncoder(os.Stdout).Encode(statusJSONOutput{
				Durable:    []durableStatusOutput{},
				Playground: []playgroundStatusOutput{},
			})
		}
		fmt.Println("No sessions bound to this Claude Code instance.")
		fmt.Println("Start one with /jamsesh:jam.")
		return nil
	}

	portalURL, err := state.ReadPortalURL()
	if err != nil {
		return fmt.Errorf("resolving portal URL: %w", err)
	}

	var durables []durableStatusOutput
	var playgrounds []playgroundStatusOutput

	for _, sessID := range sessions {
		tokenBytes, err := state.ReadSessionToken(sessID)
		if err != nil {
			// Token missing — warn and skip; don't fail the whole command.
			fmt.Fprintf(os.Stderr, "warning: no token for session %s (skipping)\n", sessID)
			continue
		}
		bearer := strings.TrimSpace(string(tokenBytes))

		orgID, yourRef := readSessionState(sessID)

		if orgID == playgroundOrgID {
			// Playground session.
			summary, err := portalclient.GetJSONWithBearer[openapi.PlaygroundSessionSummary](
				ctx, nil, portalURL,
				fmt.Sprintf("/api/playground/sessions/%s", sessID),
				bearer,
			)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: status fetch failed for playground session %s: %v\n", sessID, err)
				continue
			}
			nickname := readNickname(sessID)
			playgrounds = append(playgrounds, playgroundStatusOutput{
				SessionID:   sessID,
				OrgID:       playgroundOrgID,
				Name:        summary.Name,
				Nickname:    nickname,
				HardCapAt:   summary.HardCapAt,
				IdleTimeout: summary.IdleTimeoutAt,
				Members:     summary.MembersCount,
				Status:      string(summary.Status),
			})
		} else {
			// Durable session — orgID must be known from state.
			if orgID == "" {
				fmt.Fprintf(os.Stderr, "warning: no org_id for session %s (skipping)\n", sessID)
				continue
			}
			session, err := portalclient.GetJSONWithBearer[openapi.Session](
				ctx, nil, portalURL,
				fmt.Sprintf("/api/orgs/%s/sessions/%s", orgID, sessID),
				bearer,
			)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: status fetch failed for durable session %s: %v\n", sessID, err)
				continue
			}
			refList, err := portalclient.GetJSONWithBearer[openapi.RefListResponse](
				ctx, nil, portalURL,
				fmt.Sprintf("/api/orgs/%s/sessions/%s/refs", orgID, sessID),
				bearer,
			)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: refs fetch failed for session %s: %v\n", sessID, err)
				// Continue with empty refs rather than skipping.
				refList = openapi.RefListResponse{}
			}
			durables = append(durables, durableStatusOutput{
				SessionID: sessID,
				OrgID:     orgID,
				Name:      session.Name,
				Goal:      session.Goal,
				Mode:      string(session.DefaultMode),
				YourRef:   yourRef,
				Refs:      refList.Refs,
				Members:   session.Members,
				Comments:  []openapi.Comment{},
			})
		}
	}

	if asJSON {
		out := statusJSONOutput{
			Durable:    durables,
			Playground: playgrounds,
		}
		if out.Durable == nil {
			out.Durable = []durableStatusOutput{}
		}
		if out.Playground == nil {
			out.Playground = []playgroundStatusOutput{}
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}

	printStatusGrouped(durables, playgrounds)
	return nil
}

// printStatusGrouped writes grouped human-readable status to stdout.
func printStatusGrouped(durables []durableStatusOutput, playgrounds []playgroundStatusOutput) {
	if len(durables) > 0 {
		fmt.Println("== Durable sessions ==")
		for _, d := range durables {
			membersCount := len(d.Members)
			fmt.Printf("%-20s  Org: %-20s  Members: %d   Ref: %s\n",
				d.SessionID, d.OrgID, membersCount, d.YourRef)
		}
	}
	if len(playgrounds) > 0 {
		if len(durables) > 0 {
			fmt.Println()
		}
		fmt.Println("== Playground sessions ==")
		for _, p := range playgrounds {
			endsIn := endsInString(p.HardCapAt, p.IdleTimeout)
			fmt.Printf("%-20s  Nickname: %-20s  Ends in: %s   Members: %d\n",
				p.SessionID, p.Nickname, endsIn, p.Members)
		}
	}
	if len(durables) == 0 && len(playgrounds) == 0 {
		fmt.Println("No sessions reported status.")
	}
}

// endsInString returns a human-readable "X h Y m" until the earlier of the
// two deadline timestamps. Zero-value times are ignored.
func endsInString(hardCap, idleTimeout time.Time) string {
	var deadline time.Time
	switch {
	case hardCap.IsZero() && idleTimeout.IsZero():
		return "unknown"
	case hardCap.IsZero():
		deadline = idleTimeout
	case idleTimeout.IsZero():
		deadline = hardCap
	default:
		if hardCap.Before(idleTimeout) {
			deadline = hardCap
		} else {
			deadline = idleTimeout
		}
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return "ended"
	}
	hours := int(math.Floor(remaining.Hours()))
	minutes := int(math.Floor(remaining.Minutes())) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// readSessionState reads orgID and your ref from the per-session state directory.
// Missing files silently return empty strings; callers must handle empty orgID.
func readSessionState(sessionID string) (orgID, yourRef string) {
	dir, err := state.PluginDataDir()
	if err != nil {
		return "", ""
	}
	sessDir := filepath.Join(dir, "sessions", sessionID)

	if data, err := os.ReadFile(filepath.Join(sessDir, "org_id")); err == nil {
		orgID = strings.TrimSpace(string(data))
	}
	if data, err := os.ReadFile(filepath.Join(sessDir, "ref")); err == nil {
		yourRef = strings.TrimSpace(string(data))
	}
	return orgID, yourRef
}

// readNickname returns the joiner's nickname for a playground session.
// If the sidecar file is absent, an empty string is returned.
func readNickname(sessionID string) string {
	dir, err := state.PluginDataDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, "sessions", sessionID, "nickname"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

