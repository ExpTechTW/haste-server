package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/YuYu1015/haste-server/internal/store"
)

// languageRe bounds the language hint to something that can safely be echoed
// into JSON and matched against a highlighter grammar name.
var languageRe = regexp.MustCompile(`^[a-z0-9][a-z0-9+#._-]{0,31}$`)

type createRequest struct {
	Content  string `json:"content"`
	Language string `json:"language"`
}

type pasteResponse struct {
	Key         string `json:"key"`
	URL         string `json:"url"`
	RawURL      string `json:"rawUrl"`
	DownloadURL string `json:"downloadUrl"`
	Filename    string `json:"filename"`

	Language string  `json:"language,omitempty"`
	Content  string  `json:"content,omitempty"`
	Chars    int     `json:"chars"`
	Bytes    int     `json:"bytes"`
	Stored   int     `json:"stored"`
	Ratio    float64 `json:"ratio"`
	// No expiry is published, because none can be honoured: a paste can be
	// evicted as soon as the store needs its bytes back.
	CreatedAt time.Time `json:"createdAt"`
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleConfig lets the frontend enforce the same limits the server does,
// instead of hard-coding a copy that can drift. Retention is deliberately not
// published: the client must not show a lifetime the server cannot promise.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, map[string]any{"maxChars": s.cfg.MaxChars})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.Stats(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	ratio := 0.0
	if st.StoredSize > 0 {
		ratio = float64(st.RawBytes) / float64(st.StoredSize)
	}
	resp := map[string]any{
		"count":       st.Count,
		"rawBytes":    st.RawBytes,
		"storedBytes": st.StoredSize,
		"ratio":       round2(ratio),
	}
	if st.MaxBytes > 0 {
		resp["maxBytes"] = st.MaxBytes
		resp["usedFraction"] = round2(float64(st.StoredSize) / float64(st.MaxBytes))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	p, content, ok := s.create(w, r)
	if !ok {
		return
	}
	resp := s.describe(r, p)
	resp.Content = content
	w.Header().Set("Location", resp.URL)
	writeJSON(w, http.StatusCreated, resp)
}

// handleLegacyCreate speaks the original haste-server protocol: raw body in,
// bare {"key": ...} out.
func (s *Server) handleLegacyCreate(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.create(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": p.Code})
}

// create is the shared body-parsing and storage path for both create routes.
func (s *Server) create(w http.ResponseWriter, r *http.Request) (*store.Paste, string, bool) {
	if !s.limiter.allow(clientIP(r, s.cfg.TrustProxy)) {
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many pastes, slow down")
		return nil, "", false
	}

	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes())
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large",
				"paste exceeds the character limit")
			return nil, "", false
		}
		writeError(w, http.StatusBadRequest, "bad_request", "could not read request body")
		return nil, "", false
	}

	var req createRequest
	// A JSON content type means an envelope; anything else is the paste itself,
	// which is what `curl --data-binary @file` and the original client send.
	if isJSON(r.Header.Get("Content-Type")) {
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_json", "request body is not valid JSON")
			return nil, "", false
		}
	} else {
		req.Content = string(body)
		req.Language = r.URL.Query().Get("language")
	}

	p, err := s.store.Create(r.Context(), req.Content, normalizeLanguage(req.Language))
	if err != nil {
		s.fail(w, err)
		return nil, "", false
	}
	s.log.Info("paste created", "code", p.Code, "chars", p.Chars, "stored", p.StoredSize)
	return p, req.Content, true
}

func (s *Server) handleRead(w http.ResponseWriter, r *http.Request) {
	p, content, err := s.store.Get(r.Context(), r.PathValue("code"))
	if err != nil {
		s.fail(w, err)
		return
	}
	resp := s.describe(r, p)
	resp.Content = content
	noIndex(w)
	// Pastes are immutable, so a hit can be cached until it expires.
	w.Header().Set("Cache-Control", "public, max-age=300, immutable")
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleLegacyRead(w http.ResponseWriter, r *http.Request) {
	p, content, err := s.store.Get(r.Context(), r.PathValue("code"))
	if err != nil {
		s.fail(w, err)
		return
	}
	noIndex(w)
	writeJSON(w, http.StatusOK, map[string]string{"key": p.Code, "data": content})
}

func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	_, content, err := s.store.Get(r.Context(), r.PathValue("code"))
	if err != nil {
		s.fail(w, err)
		return
	}
	noIndex(w)
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	// The body is attacker-supplied text served from this origin. nosniff plus
	// a sandboxed, script-free CSP keeps a paste of HTML inert.
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	h.Set("Content-Disposition", "inline")
	h.Set("Cache-Control", "public, max-age=300, immutable")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, content)
}

// handleDownload serves the paste as a file named after its share code, with
// the extension its language implies, so a saved paste opens in the right mode.
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	p, content, err := s.store.Get(r.Context(), r.PathValue("code"))
	if err != nil {
		s.fail(w, err)
		return
	}

	// Both halves come from values this server controls — a base62 code and a
	// fixed extension table — so the filename needs no further escaping.
	filename := p.Code + "." + extensionFor(p.Language)

	noIndex(w)
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cache-Control", "public, max-age=300, immutable")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, content)
}

// describe renders shared response metadata, including the compression ratio
// actually achieved for this paste.
func (s *Server) describe(r *http.Request, p *store.Paste) pasteResponse {
	base := s.baseURL(r)
	resp := pasteResponse{
		Key:         p.Code,
		URL:         base + "/" + p.Code,
		RawURL:      base + "/raw/" + p.Code,
		DownloadURL: base + "/download/" + p.Code,
		Filename:    p.Code + "." + extensionFor(p.Language),
		Language:    p.Language,
		Chars:       p.Chars,
		Bytes:       p.RawBytes,
		Stored:      p.StoredSize,
		CreatedAt:   p.CreatedAt,
	}
	if p.StoredSize > 0 {
		resp.Ratio = round2(float64(p.RawBytes) / float64(p.StoredSize))
	}
	return resp
}

// baseURL prefers the configured origin; otherwise it reconstructs one from
// the request so the server works unconfigured behind any hostname.
func (s *Server) baseURL(r *http.Request) string {
	if s.cfg.BaseURL != "" {
		return s.cfg.BaseURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if s.cfg.TrustProxy {
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		}
	}
	return scheme + "://" + r.Host
}

// fail maps a storage error onto the right status code.
func (s *Server) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no paste with that code")
	case errors.Is(err, store.ErrEmpty):
		writeError(w, http.StatusBadRequest, "empty", "paste is empty")
	case errors.Is(err, store.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", err.Error())
	case errors.Is(err, store.ErrBusy):
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusServiceUnavailable, "busy",
			"the server is saving too many pastes right now, try again shortly")
	case errors.Is(err, store.ErrNoRoom):
		writeError(w, http.StatusInsufficientStorage, "no_room",
			"the server's storage cap is smaller than this paste")
	default:
		s.log.Error("request failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
	}
}

// noIndex asks search engines not to list a paste, while still allowing the
// fetch itself. robots.txt cannot draw that line: refusing the crawl there also
// refuses the crawlers that build link previews.
func noIndex(w http.ResponseWriter) {
	w.Header().Set("X-Robots-Tag", "noindex")
}

func normalizeLanguage(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if !languageRe.MatchString(s) {
		return ""
	}
	return s
}

func isJSON(contentType string) bool {
	mediaType, _, _ := strings.Cut(contentType, ";")
	return strings.EqualFold(strings.TrimSpace(mediaType), "application/json")
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: code, Message: message})
}
