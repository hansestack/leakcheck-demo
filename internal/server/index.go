package server

import (
	"log/slog"
	"net/http"
)

// slogError records a response-write failure. The status line is already
// committed at that point, so logging is the only remaining action.
func slogError(err error) {
	slog.Error("failed to write response", "err", err)
}

// indexHTML is a minimal form so the demo can be exercised from a browser.
const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Hansestack leak-check demo</title>
  <style>
    body { font-family: system-ui, sans-serif; max-width: 40rem; margin: 4rem auto; padding: 0 1rem; }
    input, button { font: inherit; padding: .5rem; margin: .25rem 0; width: 100%; box-sizing: border-box; }
    pre { background: #f4f4f5; padding: 1rem; overflow-x: auto; }
  </style>
</head>
<body>
  <h1>Hansestack leak-check demo</h1>
  <p>Try <code>password</code> or <code>hunter2</code> for a leaked result.</p>
  <form id="f">
    <input name="email" type="email" placeholder="email" value="demo@example.com" required>
    <input name="password" type="text" placeholder="password" value="hunter2" required>
    <button type="submit">Sign up</button>
  </form>
  <pre id="out">awaiting submission…</pre>
  <script>
    document.getElementById('f').addEventListener('submit', async (e) => {
      e.preventDefault();
      const data = Object.fromEntries(new FormData(e.target));
      const res  = await fetch('/signup', {
        method:  'POST',
        headers: { 'Content-Type': 'application/json' },
        body:    JSON.stringify(data),
      });
      document.getElementById('out').textContent =
        'HTTP ' + res.status + '\n' + JSON.stringify(await res.json(), null, 2);
    });
  </script>
</body>
</html>`

// handleIndex serves a small HTML form for manual exploration.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// The catch-all pattern "GET /" matches every unrouted path.
	if r.URL.Path != "/" {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte(indexHTML)); err != nil {
		slogError(err)
	}
}
