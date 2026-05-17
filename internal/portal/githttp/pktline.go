package githttp

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"jamsesh/internal/portal/prereceive"
)

// readPktLineLen reads the 4-hex-digit length prefix from r and returns the
// decoded total packet length (including the 4-byte prefix). Returns 0 for
// the flush packet ("0000").
func readPktLineLen(r io.Reader) (int, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, fmt.Errorf("pktline: read length prefix: %w", err)
	}

	var n int
	for _, c := range buf {
		n <<= 4
		switch {
		case c >= '0' && c <= '9':
			n |= int(c - '0')
		case c >= 'a' && c <= 'f':
			n |= int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			n |= int(c-'A') + 10
		default:
			return 0, fmt.Errorf("pktline: invalid hex digit %q", c)
		}
	}
	return n, nil
}

// readCommandList parses the receive-pack command list from r.
// Each line is "<old-sha> <new-sha> <ref-name>\0<capabilities>" (NUL + caps
// only on the first line). The list ends with "0000" flush.
//
// Returns the slice of RefUpdates and a reader whose unread bytes are the
// pack data (everything after the flush packet).
func readCommandList(r io.Reader) (updates []prereceive.RefUpdate, packReader io.Reader, err error) {
	br := bufio.NewReader(r)

	for {
		n, err := readPktLineLen(br)
		if err != nil {
			return nil, nil, err
		}
		if n == 0 {
			// "0000" flush packet — command list is done; remainder is pack data.
			return updates, br, nil
		}
		// n includes the 4-byte length prefix; payload length = n - 4.
		if n < 4 {
			return nil, nil, fmt.Errorf("pktline: invalid packet length %d", n)
		}
		payloadLen := n - 4
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(br, payload); err != nil {
			return nil, nil, fmt.Errorf("pktline: read payload: %w", err)
		}

		// Strip capabilities (NUL-separated, first line only) and trailing newline.
		line := string(payload)
		if idx := strings.IndexByte(line, 0); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSuffix(line, "\n")

		// Parse: "<40-hex-sha> <40-hex-sha> <ref>"
		parts := strings.SplitN(line, " ", 3)
		if len(parts) != 3 {
			return nil, nil, fmt.Errorf("pktline: malformed command line %q", line)
		}
		oldSHA := parts[0]
		// The git wire protocol uses 40 zero digits to signal "ref does not
		// exist yet" (new ref creation). Map that to an empty string so that
		// downstream prereceive logic treats it as a new-ref case (no
		// ancestor check, no force-push check).
		if oldSHA == "0000000000000000000000000000000000000000" {
			oldSHA = ""
		}
		updates = append(updates, prereceive.RefUpdate{
			OldSHA: oldSHA,
			NewSHA: parts[1],
			Ref:    parts[2],
		})
	}
}

// writePktLine writes a single pkt-line to w. The length prefix includes the
// 4-byte prefix itself.
func writePktLine(w io.Writer, payload string) error {
	_, err := fmt.Fprintf(w, "%04x%s", len(payload)+4, payload)
	return err
}

// writeFlushPkt writes the "0000" flush packet to w.
func writeFlushPkt(w io.Writer) error {
	_, err := io.WriteString(w, "0000")
	return err
}

// writeReportStatusRejection writes a smart-HTTP report-status payload
// indicating that all refs were rejected. The format matches what git clients
// expect to render inline rejection messages:
//
//	0014unpack ok\n
//	<NNNN>ng <ref-name> <reason>\n
//	...
//	0000
//
// If there are no specific per-ref rejections but validationFailed is true,
// a generic message is used for each update.
func writeReportStatusRejection(w io.Writer, updates []prereceive.RefUpdate, rejections []prereceive.Rejection) {
	// unpack ok — the pack itself parsed correctly; only refs are rejected.
	_ = writePktLine(w, "unpack ok\n")

	// Build a map from ref → first rejection message for per-ref reporting.
	// If a rejection has no ref detail, we apply it to all updates.
	perRef := make(map[string]string)
	var globalReason string
	for _, r := range rejections {
		if ref, ok := r.Details["ref"].(string); ok && ref != "" {
			if _, exists := perRef[ref]; !exists {
				perRef[ref] = r.Message
			}
		} else {
			if globalReason == "" {
				globalReason = r.Message
			}
		}
	}

	for _, u := range updates {
		reason := perRef[u.Ref]
		if reason == "" {
			reason = globalReason
		}
		if reason == "" {
			reason = "rejected by server policy"
		}
		_ = writePktLine(w, "ng "+u.Ref+" "+reason+"\n")
	}

	_ = writeFlushPkt(w)
}
