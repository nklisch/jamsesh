package mailhog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Message is a minimal subset of MailHog's V2 message shape, normalised for
// easy test assertions.
type Message struct {
	// ID is MailHog's internal message identifier.
	ID string
	// From is the envelope sender formatted as "mailbox@domain".
	From string
	// To is the list of envelope recipients, each as "mailbox@domain".
	To []string
	// Subject is the message subject header.
	Subject string
	// Body is the raw text body of the message.
	Body string
	// CreatedAt is the time MailHog recorded the message.
	CreatedAt time.Time
}

// LatestMessageTo polls /api/v2/messages until it finds a message addressed to
// the given recipient, then returns the most recent one. It retries every 100ms
// until timeout (default 5s if 0 is passed). The test is failed if no message
// arrives within the deadline.
func (m *MailHog) LatestMessageTo(ctx context.Context, t *testing.T, email string, timeout time.Duration) Message {
	t.Helper()
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msg, ok, err := m.fetchLatestTo(ctx, email)
		if err != nil {
			t.Fatalf("mailhog: LatestMessageTo: %v", err)
		}
		if ok {
			return msg
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("mailhog: no message to %q within %s", email, timeout)
	return Message{} // unreachable — t.Fatal exits
}

// fetchLatestTo fetches all messages from MailHog and returns the most recent
// one addressed to email (matched case-insensitively). Returns (msg, true, nil)
// on success, (Message{}, false, nil) when no match, and (Message{}, false, err)
// on HTTP or decode errors.
func (m *MailHog) fetchLatestTo(ctx context.Context, email string) (Message, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.HTTPURL+"/api/v2/messages", nil)
	if err != nil {
		return Message{}, false, fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Message{}, false, fmt.Errorf("GET /api/v2/messages: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return Message{}, false, fmt.Errorf("GET /api/v2/messages: status %d: %s", resp.StatusCode, body)
	}

	var envelope struct {
		Items []struct {
			ID   string `json:"ID"`
			From struct {
				Mailbox string `json:"Mailbox"`
				Domain  string `json:"Domain"`
			} `json:"From"`
			To []struct {
				Mailbox string `json:"Mailbox"`
				Domain  string `json:"Domain"`
			} `json:"To"`
			Content struct {
				Headers map[string][]string `json:"Headers"`
				Body    string              `json:"Body"`
			} `json:"Content"`
			Created string `json:"Created"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return Message{}, false, fmt.Errorf("decode messages: %w", err)
	}

	// MailHog returns items newest-first; iterate and return the first match.
	target := strings.ToLower(email)
	for _, item := range envelope.Items {
		for _, to := range item.To {
			addr := strings.ToLower(to.Mailbox + "@" + to.Domain)
			if addr != target {
				continue
			}
			var subject string
			if subs, ok := item.Content.Headers["Subject"]; ok && len(subs) > 0 {
				subject = subs[0]
			}
			var createdAt time.Time
			if item.Created != "" {
				createdAt, _ = time.Parse(time.RFC3339, item.Created)
			}
			from := item.From.Mailbox + "@" + item.From.Domain
			tos := make([]string, 0, len(item.To))
			for _, t := range item.To {
				tos = append(tos, t.Mailbox+"@"+t.Domain)
			}
			return Message{
				ID:        item.ID,
				From:      from,
				To:        tos,
				Subject:   subject,
				Body:      item.Content.Body,
				CreatedAt: createdAt,
			}, true, nil
		}
	}
	return Message{}, false, nil
}
