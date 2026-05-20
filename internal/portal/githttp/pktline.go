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
// Returns the slice of RefUpdates, the capability set advertised by the client
// on the first command line (e.g. "side-band-64k"), and a reader whose unread
// bytes are the pack data (everything after the flush packet).
func readCommandList(r io.Reader) (updates []prereceive.RefUpdate, caps map[string]bool, packReader io.Reader, err error) {
	br := bufio.NewReader(r)
	caps = make(map[string]bool)

	for {
		n, err := readPktLineLen(br)
		if err != nil {
			return nil, nil, nil, err
		}
		if n == 0 {
			// "0000" flush packet — command list is done; remainder is pack data.
			return updates, caps, br, nil
		}
		// n includes the 4-byte length prefix; payload length = n - 4.
		if n < 4 {
			return nil, nil, nil, fmt.Errorf("pktline: invalid packet length %d", n)
		}
		payloadLen := n - 4
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(br, payload); err != nil {
			return nil, nil, nil, fmt.Errorf("pktline: read payload: %w", err)
		}

		// Parse capabilities from the NUL-separated suffix on the first line only.
		line := string(payload)
		if idx := strings.IndexByte(line, 0); idx >= 0 {
			capPart := strings.TrimSuffix(line[idx+1:], "\n")
			for _, cap := range strings.Fields(capPart) {
				caps[cap] = true
			}
			line = line[:idx]
		}
		line = strings.TrimSuffix(line, "\n")

		// Parse: "<40-hex-sha> <40-hex-sha> <ref>"
		parts := strings.SplitN(line, " ", 3)
		if len(parts) != 3 {
			return nil, nil, nil, fmt.Errorf("pktline: malformed command line %q", line)
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

// writeSidebandPktLine writes an outer sideband-wrapped pkt-line to w.
// band must be 1 (data), 2 (progress), or 3 (error). The outer packet
// wraps a single band byte followed by the inner payload:
//
//	<outer-len><band-byte><inner-payload>
//
// where outer-len covers the 4-byte length prefix, the 1-byte band prefix,
// and the inner payload bytes.
func writeSidebandPktLine(w io.Writer, band byte, payload string) error {
	// outer length = 4 (length prefix) + 1 (band byte) + len(payload)
	outerLen := 4 + 1 + len(payload)
	_, err := fmt.Fprintf(w, "%04x", outerLen)
	if err != nil {
		return err
	}
	if _, err = w.Write([]byte{band}); err != nil {
		return err
	}
	_, err = io.WriteString(w, payload)
	return err
}

// writeFlushPkt writes the "0000" flush packet to w.
func writeFlushPkt(w io.Writer) error {
	_, err := io.WriteString(w, "0000")
	return err
}

// writeReportStatusRejection writes a smart-HTTP report-status payload
// indicating that all refs were rejected. The format matches what git clients
// expect to render inline rejection messages.
//
// When caps includes "side-band-64k", each inner pkt-line is wrapped in an
// outer sideband packet on band 1 (data channel):
//
//	<outer-len>\x01<inner-pkt-line>
//	...
//	0000
//
// Without sideband negotiation the inner pkt-lines are written directly:
//
//	0014unpack ok\n
//	<NNNN>ng <ref-name> <reason>\n
//	...
//	0000
//
// If there are no specific per-ref rejections a generic message is used for
// each update.
func writeReportStatusRejection(w io.Writer, updates []prereceive.RefUpdate, rejections []prereceive.Rejection, caps map[string]bool) {
	sideband := caps["side-band-64k"]

	writeLine := func(payload string) {
		if sideband {
			// Inner pkt-line bytes: 4-byte length prefix + payload
			inner := fmt.Sprintf("%04x%s", len(payload)+4, payload)
			_ = writeSidebandPktLine(w, 0x01, inner)
		} else {
			_ = writePktLine(w, payload)
		}
	}

	// unpack ok — the pack itself parsed correctly; only refs are rejected.
	writeLine("unpack ok\n")

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
		writeLine("ng " + u.Ref + " " + reason + "\n")
	}

	_ = writeFlushPkt(w)
}
