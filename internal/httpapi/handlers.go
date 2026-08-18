package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/YuYu1015/haste-server/internal/config"
	"github.com/YuYu1015/haste-server/internal/store"
)

// languageRe bounds the language hint to something that can safely be echoed
// into JSON and matched against a highlighter grammar name.
var languageRe = regexp.MustCompile(`^[a-z0-9][a-z0-9+#._-]{0,31}$`)

type createRequest struct {
	Content  string `json:"content"`
	Language string `json:"language"`
	// ExpiresIn is a lifetime in seconds. Absent or 0 means none is requested;
	// anything else has to fall inside [store.MinTTL, store.MaxTTL].
	ExpiresIn int64 `json:"expiresIn"`
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

	CreatedAt time.Time `json:"createdAt"`
	// ExpiresAt is present only when a lifetime was asked for at save time.
	// Its absence is not a promise of permanence — the storage cap can still
	// reclaim any paste — which is why the field is omitted rather than sent as
	// null with a meaning attached to it.
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// expiryOptionsSecs is the lifetime ladder as the API publishes it. Built once,
// because it never changes and /api/config is on the first-paint path.
var expiryOptionsSecs = func() []int64 {
	out := make([]int64, len(store.TTLOptions))
	for i, d := range store.TTLOptions {
		out[i] = int64(d.Seconds())
	}
	return out
}()

// badExpiryMessage is what a rejected lifetime gets told. It names the accepted
// values outright rather than describing a rule, because the field takes
// seconds and the list is short enough to just print.
var badExpiryMessage = func() string {
	secs := make([]string, len(expiryOptionsSecs))
	for i, n := range expiryOptionsSecs {
		secs[i] = strconv.FormatInt(n, 10)
	}
	return "expiresIn must be 0 (no expiry) or one of " + strings.Join(secs, ", ") +
		" seconds; the query string also accepts these as durations, e.g. 6h or 30d"
}()

// handleConfig lets the frontend enforce the same limits the server does,
// instead of hard-coding a copy that can drift.
//
// The lifetime ladder is published rather than described, so the picker is
// built from the same list Create validates against and the two cannot disagree
// about what is on offer. The base URL is published so the API reference can
// show the address people should actually call, which behind a proxy is not the
// one the browser happens to be on. No default retention is published, because a paste
// that asks for nothing gets no promise: the storage cap can reclaim it
// whenever it needs the bytes.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=60")
	cfg := map[string]any{
		"maxChars":          s.cfg.MaxChars,
		"expiryOptionsSecs": expiryOptionsSecs,
		"cleanupEverySecs":  int64(s.cfg.CleanupInterval.Seconds()),
	}
	// Only when an operator has actually declared one. Deriving it from the
	// request instead would make this response vary by Host while the cache
	// keys it by path alone, and the client can read its own origin anyway.
	if s.cfg.BaseURL != "" {
		cfg["baseUrl"] = s.cfg.BaseURL
	}
	// Only the public mode. A token-gated endpoint is for the operator's
	// monitoring, and documenting it to strangers would only invite guesses.
	if s.cfg.Stats == config.StatsPublic {
		cfg["statsPublic"] = true
	}
	writeJSON(w, http.StatusOK, cfg)
}

// statsCacheFor bounds how fresh /api/stats can be.
//
// It does two jobs at once. The query is a full table scan — 29ms against
// 300k pastes, against 0.2ms for every other endpoint — so without a cache an
// unauthenticated caller can spend the server's IO a hundred times over per
// request. And the uncached numbers move one paste at a time, which makes them
// a size oracle for every paste as it arrives; a window coarsens that to a
// bucket. Short enough that a dashboard is still live.
const statsCacheFor = 10 * time.Second

// handleStats reports the corpus totals, to whoever the operator allows.
//
// Off by default. The totals are of far more use to someone attacking the
// instance than to anyone using it: they turn filling the storage cap into a
// task with a progress bar, and a falling count is a receipt confirming that
// other people's pastes have been evicted.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	switch s.cfg.Stats {
	case config.StatsPublic:
		// Cacheable by anyone, for as long as the server itself would reuse it.
		w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(int(statsCacheFor.Seconds())))
	case config.StatsToken:
		if !s.statsAuthorized(r) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="stats"`)
			writeError(w, http.StatusUnauthorized, "unauthorized", "stats requires a bearer token")
			return
		}
		// An authorized body must not sit in a shared cache.
		w.Header().Set("Cache-Control", "no-store")
	default:
		// 404 rather than 403: a disabled endpoint should look like an absent
		// one, and say nothing about whether stats exist to be unlocked.
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
		return
	}

	st, err := s.cachedStats(r.Context())
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

// statsAuthorized checks the bearer token in constant time, so a wrong guess
// takes as long as a right one whatever prefix it shares.
func (s *Server) statsAuthorized(r *http.Request) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.StatsToken)) == 1
}

// cachedStats reuses a recent scan rather than repeating it per request.
func (s *Server) cachedStats(ctx context.Context) (store.Stats, error) {
	now := time.Now()

	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	if now.Sub(s.statsAt) < statsCacheFor {
		return s.statsValue, nil
	}

	st, err := s.store.Stats(ctx)
	if err != nil {
		return store.Stats{}, err
	}
	s.statsValue, s.statsAt = st, now
	return st, nil
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
		// The raw path has no envelope to carry a lifetime, so the query string
		// does: ?expiresIn=1h for a person typing curl, ?expiresIn=3600 for a
		// script that would rather not build a duration string.
		ttl, err := parseExpiresIn(r.URL.Query().Get("expiresIn"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_expiry", err.Error())
			return nil, "", false
		}
		req.ExpiresIn = int64(ttl.Seconds())
	}

	p, err := s.store.Create(r.Context(), req.Content, normalizeLanguage(req.Language),
		time.Duration(req.ExpiresIn)*time.Second)
	if err != nil {
		s.fail(w, err)
		return nil, "", false
	}
	s.log.Info("paste created",
		"code", p.Code, "chars", p.Chars, "stored", p.StoredSize, "expires", p.ExpiresAt)
	return p, req.Content, true
}

// daysRe matches the one duration unit Go does not have.
//
// The picker calls the longer rungs "7 days" and "30 days", so 7d and 30d are
// what someone reaches for at a shell prompt — and time.ParseDuration rejects
// both, having no unit above the hour.
var daysRe = regexp.MustCompile(`^(\d{1,4})d$`)

// parseExpiresIn reads a lifetime written as a plain count of seconds, as a Go
// duration, or in days. Empty means none was asked for.
func parseExpiresIn(v string) (time.Duration, error) {
	if v == "" {
		return 0, nil
	}
	if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
		return time.Duration(secs) * time.Second, nil
	}
	if m := daysRe.FindStringSubmatch(v); m != nil {
		days, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, errors.New(badExpiryMessage)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, errors.New(badExpiryMessage)
	}
	return d, nil
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
	setCacheControl(w, p)
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
	paste, content, err := s.store.Get(r.Context(), r.PathValue("code"))
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
	setCacheControl(w, paste)
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
	setCacheControl(w, p)
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
	if !p.ExpiresAt.IsZero() {
		at := p.ExpiresAt
		resp.ExpiresAt = &at
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
	case errors.Is(err, store.ErrBadTTL):
		writeError(w, http.StatusBadRequest, "bad_expiry", badExpiryMessage)
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

// cacheSeconds is how long a paste may be held by a shared cache. Pastes are
// immutable, so the only thing that can change under a URL is the paste ceasing
// to exist.
const cacheSeconds = 300

// setCacheControl caches a paste for as long as it is certain to still be there.
//
// A paste that expires in forty seconds must not be cached for five minutes:
// the edge would keep serving a link the server has already retired. Trimming
// the lifetime to whatever is left is what makes the expiry hold everywhere and
// not just at the origin.
func setCacheControl(w http.ResponseWriter, p *store.Paste) {
	age := cacheSeconds
	if p != nil && !p.ExpiresAt.IsZero() {
		if left := int(time.Until(p.ExpiresAt).Seconds()); left < age {
			age = max(left, 0)
		}
	}
	if age == 0 {
		w.Header().Set("Cache-Control", "no-store")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(age)+", immutable")
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
