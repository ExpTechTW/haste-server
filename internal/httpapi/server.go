// Package httpapi serves the JSON API, the raw-text endpoints, and the
// embedded single-page frontend.
package httpapi

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/YuYu1015/haste-server/internal/config"
	"github.com/YuYu1015/haste-server/internal/store"
)

// ReservedCodes are the top-level paths the server owns. The code generator
// refuses to issue any of them, so a share link can never be shadowed by a
// route — and a future route can never be shadowed by an existing paste.
var ReservedCodes = []string{
	"api", "raw", "download", "documents", "assets", "healthz", "health",
	"about", "new", "docs", "static", "index", "favicon", "robots", "manifest",
}

// Server wires configuration, storage, and the embedded UI into one handler.
type Server struct {
	cfg     *config.Config
	store   *store.Store
	ui      fs.FS
	uiTags  map[string]string
	log     *slog.Logger
	limiter *ipLimiter
}

// New builds the server. ui is the root of the built frontend.
func New(cfg *config.Config, st *store.Store, ui fs.FS, log *slog.Logger) *Server {
	return &Server{
		cfg:     cfg,
		store:   st,
		ui:      ui,
		uiTags:  buildETags(ui),
		log:     log,
		limiter: newIPLimiter(cfg.RateRPS, cfg.RateBurst),
	}
}

// buildETags hashes every embedded file once at startup.
//
// Embedded files carry no modification time, so net/http omits Last-Modified
// and a revalidation has nothing to compare against — every conditional
// request comes back as a full body. A content hash gives caches something to
// ask about, which is what turns the frontend's no-cache shell and its
// short-lived favicon into 304s instead of re-downloads.
func buildETags(ui fs.FS) map[string]string {
	tags := make(map[string]string)
	err := fs.WalkDir(ui, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		body, err := fs.ReadFile(ui, path)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(body)
		// 12 bytes is far past the point where a collision is plausible for a
		// few dozen files, and keeps the header short.
		tags[path] = `"` + base64.RawURLEncoding.EncodeToString(sum[:12]) + `"`
		return nil
	})
	if err != nil {
		return tags
	}
	return tags
}

// Handler returns the fully wrapped HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealth)

	// Everything that returns a body worth caching or compressing goes through
	// buffered(): it adds the ETag and the gzip. The static handler is left out
	// on purpose — it already carries build-time tags and serves byte ranges,
	// which buffering would break.
	mux.HandleFunc("GET /api/config", buffered(s.handleConfig))
	mux.HandleFunc("GET /api/stats", buffered(s.handleStats))
	mux.HandleFunc("POST /api/pastes", buffered(s.handleCreate))
	mux.HandleFunc("GET /api/pastes/{code}", buffered(s.handleRead))
	mux.HandleFunc("GET /raw/{code}", buffered(s.handleRaw))
	mux.HandleFunc("GET /download/{code}", buffered(s.handleDownload))

	// haste-server wire compatibility, so existing CLI wrappers keep working.
	mux.HandleFunc("POST /documents", buffered(s.handleLegacyCreate))
	mux.HandleFunc("GET /documents/{code}", buffered(s.handleLegacyRead))

	// Everything else is the SPA: static assets when the path names one,
	// index.html otherwise, which is how /{code} reaches the client router.
	mux.HandleFunc("GET /", s.handleUI)

	return s.recoverer(s.logging(s.securityHeaders(s.cors(mux))))
}

// recoverer keeps one panicking request from taking the process with it.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.log.Error("panic serving request", "method", r.Method, "path", r.URL.Path, "panic", v)
				writeError(w, http.StatusInternalServerError, "internal", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		s.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"bytes", sw.bytes,
			"duration", time.Since(start).Round(time.Microsecond).String(),
		)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	allowAll := false
	allowed := make(map[string]struct{}, len(s.cfg.CORSOrigins))
	for _, o := range s.cfg.CORSOrigins {
		if o == "*" {
			allowAll = true
		}
		allowed[o] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			_, ok := allowed[origin]
			switch {
			case allowAll:
				// No credentials are ever involved, so a wildcard is safe here.
				w.Header().Set("Access-Control-Allow-Origin", "*")
			case ok:
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			}
		}
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			h := w.Header()
			h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type")
			h.Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleUI serves the built frontend. Unknown paths fall through to index.html
// so client-side routes such as /{code} survive a hard refresh.
func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || name == "." {
		s.serveIndex(w, r)
		return
	}

	f, err := s.ui.Open(name)
	if err != nil {
		s.serveIndex(w, r)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		s.serveIndex(w, r)
		return
	}
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		s.serveIndex(w, r)
		return
	}

	// Vite fingerprints asset filenames, so their contents can never change
	// under a given URL — anything else has to be revalidated.
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	// ServeContent answers If-None-Match itself once the tag is set.
	if tag, ok := s.uiTags[name]; ok {
		w.Header().Set("Etag", tag)
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), rs)
}

// uiMissingPage stands in when the binary was built without running the
// frontend build. The API is fully functional in that state, so this explains
// the gap rather than failing opaquely.
const uiMissingPage = `<!doctype html><meta charset="utf-8"><title>haste</title>
<style>body{font:14px/1.6 ui-monospace,Menlo,monospace;margin:6rem auto;max-width:34rem;padding:0 1.5rem}
code{background:#8881;border-radius:4px;padding:.15rem .35rem}</style>
<h1>Frontend not built</h1>
<p>The API is running. Build the web assets and recompile:</p>
<p><code>make build</code></p>
<p>The JSON API at <code>/api/pastes</code> works without it.</p>`

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	// The shell names the current asset bundle, so it must never be cached.
	h.Set("Cache-Control", "no-cache")

	b, err := fs.ReadFile(s.ui, "index.html")
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		io.WriteString(w, uiMissingPage)
		return
	}
	// no-cache means "revalidate first", not "do not store", so the tag is what
	// makes that revalidation cheap.
	if tag, ok := s.uiTags["index.html"]; ok {
		h.Set("Etag", tag)
		if match := r.Header.Get("If-None-Match"); match == tag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	w.Write(b)
}
