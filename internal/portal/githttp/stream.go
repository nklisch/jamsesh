package githttp

import "net/http"

// streamWithFlush copies from src to w in 32 KiB chunks, calling Flush after
// each write if the writer implements http.Flusher. This keeps large fetch and
// push responses from stalling the git client while data accumulates in
// OS send buffers.
func streamWithFlush(w http.ResponseWriter, src interface{ Read([]byte) (int, error) }) {
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}
