package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/YuYu1015/haste-server/internal/compress"
	"github.com/YuYu1015/haste-server/internal/config"
	"github.com/YuYu1015/haste-server/internal/id"
	"github.com/YuYu1015/haste-server/internal/store"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	codec, err := compress.New(19)
	if err != nil {
		t.Fatalf("compress.New: %v", err)
	}
	t.Cleanup(codec.Close)

	cfg := &config.Config{
		MaxChars:    4000,
		ZstdLevel:   19,
		Retention:   30 * 24 * time.Hour,
		CORSOrigins: []string{"*"},
		// Rate limiting has its own test; leave it off elsewhere so a burst of
		// table-driven cases cannot trip it.
		RateRPS: 0,
	}

	st, err := store.Open(context.Background(), store.Options{
		Path:      filepath.Join(t.TempDir(), "haste.db"),
		CacheMB:   8,
		ReadPool:  4,
		MaxChars:  cfg.MaxChars,
		Retention: cfg.Retention,
		Codec:     codec,
		IDs:       id.NewGenerator([]byte("test-secret"), id.DefaultMinLen, ReservedCodes),
	})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	ui := fstest.MapFS{
		"index.html":        &fstest.MapFile{Data: []byte("<!doctype html><div id=root></div>")},
		"assets/app.123.js": &fstest.MapFile{Data: []byte("console.log(1)")},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	srv := httptest.NewServer(New(cfg, st, ui, log).Handler())
	t.Cleanup(srv.Close)
	return srv
}

func postJSON(t *testing.T, srv *httptest.Server, body string) (*http.Response, pasteResponse) {
	t.Helper()
	resp, err := srv.Client().Post(srv.URL+"/api/pastes", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/pastes: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	var out pasteResponse
	json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func TestCreateAndFetch(t *testing.T) {
	srv := newTestServer(t)

	resp, created := postJSON(t, srv, `{"content":"hello\nworld","language":"go"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	if created.Key == "" {
		t.Fatal("response carried no key")
	}
	if !strings.HasSuffix(created.URL, "/"+created.Key) {
		t.Errorf("URL = %q, want it to end in /%s", created.URL, created.Key)
	}
	if created.Ratio <= 0 || created.Stored <= 0 {
		t.Errorf("compression metadata missing: stored=%d ratio=%v", created.Stored, created.Ratio)
	}
	if created.ExpiresAt == nil {
		t.Error("expiresAt should be set when retention is configured")
	}

	// JSON read
	r, err := srv.Client().Get(srv.URL + "/api/pastes/" + created.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var fetched pasteResponse
	json.NewDecoder(r.Body).Decode(&fetched)
	if fetched.Content != "hello\nworld" {
		t.Errorf("content = %q, want %q", fetched.Content, "hello\nworld")
	}
	if fetched.Language != "go" {
		t.Errorf("language = %q, want go", fetched.Language)
	}

	// Raw read
	raw, err := srv.Client().Get(srv.URL + "/raw/" + created.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Body.Close()
	b, _ := io.ReadAll(raw.Body)
	if string(b) != "hello\nworld" {
		t.Errorf("raw body = %q", b)
	}
	if ct := raw.Header.Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("raw Content-Type = %q", ct)
	}
	// A paste of HTML must not be renderable as a page on this origin.
	if raw.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("raw response is missing nosniff")
	}
	if !strings.Contains(raw.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Error("raw response is missing a sandboxing CSP")
	}
}

// A download must arrive as a file named after the share code, carrying the
// extension its language implies.
func TestDownloadFilename(t *testing.T) {
	srv := newTestServer(t)

	cases := []struct {
		language string
		wantExt  string
	}{
		{"dart", "dart"},
		{"go", "go"},
		{"python", "py"},
		{"typescript", "ts"},
		{"csharp", "cs"},
		{"", "txt"},         // no language given
		{"nonsense", "txt"}, // unknown language
	}

	for _, tc := range cases {
		t.Run(tc.language, func(t *testing.T) {
			_, created := postJSON(t, srv, `{"content":"body here","language":"`+tc.language+`"}`)
			want := created.Key + "." + tc.wantExt

			if created.Filename != want {
				t.Errorf("filename in response = %q, want %q", created.Filename, want)
			}

			resp, err := srv.Client().Get(srv.URL + "/download/" + created.Key)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			disposition := resp.Header.Get("Content-Disposition")
			if disposition != `attachment; filename="`+want+`"` {
				t.Errorf("Content-Disposition = %q, want an attachment named %q", disposition, want)
			}
			body, _ := io.ReadAll(resp.Body)
			if string(body) != "body here" {
				t.Errorf("downloaded body = %q", body)
			}
		})
	}
}

func TestCreateAcceptsRawBody(t *testing.T) {
	srv := newTestServer(t)

	resp, err := srv.Client().Post(srv.URL+"/api/pastes?language=python", "text/plain", strings.NewReader("print(1)"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var out pasteResponse
	json.NewDecoder(resp.Body).Decode(&out)
	if out.Language != "python" {
		t.Errorf("language = %q, want python", out.Language)
	}
	if out.Content != "print(1)" {
		t.Errorf("content = %q", out.Content)
	}
}

// The original haste-server API, so existing CLI wrappers keep working.
func TestLegacyDocumentsAPI(t *testing.T) {
	srv := newTestServer(t)

	resp, err := srv.Client().Post(srv.URL+"/documents", "text/plain", strings.NewReader("legacy body"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var created map[string]string
	json.NewDecoder(resp.Body).Decode(&created)
	key := created["key"]
	if key == "" {
		t.Fatal("no key returned")
	}

	get, err := srv.Client().Get(srv.URL + "/documents/" + key)
	if err != nil {
		t.Fatal(err)
	}
	defer get.Body.Close()
	var doc map[string]string
	json.NewDecoder(get.Body).Decode(&doc)
	if doc["data"] != "legacy body" {
		t.Errorf("data = %q", doc["data"])
	}
}

func TestRejections(t *testing.T) {
	srv := newTestServer(t)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"empty", `{"content":""}`, http.StatusBadRequest},
		{"malformed json", `{"content":`, http.StatusBadRequest},
		{"over limit", `{"content":"` + strings.Repeat("a", 4001) + `"}`, http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := postJSON(t, srv, tc.body)
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}

	// Exactly at the limit still succeeds.
	resp, _ := postJSON(t, srv, `{"content":"`+strings.Repeat("a", 4000)+`"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("4000 chars: status = %d, want 201", resp.StatusCode)
	}
}

// A body far past the limit must be refused while streaming, not buffered whole.
func TestOversizedBodyIsCutOff(t *testing.T) {
	srv := newTestServer(t)

	resp, _ := postJSON(t, srv, `{"content":"`+strings.Repeat("a", 5_000_000)+`"}`)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", resp.StatusCode)
	}
}

func TestUnknownCodeReturns404(t *testing.T) {
	srv := newTestServer(t)

	for _, path := range []string{"/api/pastes/zzzz", "/raw/zzzz", "/documents/zzzz"} {
		resp, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, resp.StatusCode)
		}
	}
}

func TestRateLimit(t *testing.T) {
	srv := newTestServer(t)
	// Reach past the constructor to exercise the limiter without a slow test.
	handler := srv.Config.Handler
	_ = handler

	limiter := newIPLimiter(1, 3)
	allowed := 0
	for i := 0; i < 10; i++ {
		if limiter.allow("10.0.0.1") {
			allowed++
		}
	}
	if allowed != 3 {
		t.Errorf("burst of 3 allowed %d requests, want 3", allowed)
	}
	if !limiter.allow("10.0.0.2") {
		t.Error("a different client should not be affected by another's burst")
	}
	if nolimit := newIPLimiter(0, 0); !nolimit.allow("10.0.0.1") {
		t.Error("rps of 0 should disable limiting")
	}
}

// Client-side routes must survive a hard refresh, and assets must still win
// over the SPA fallback.
func TestSPAFallback(t *testing.T) {
	srv := newTestServer(t)

	resp, err := srv.Client().Get(srv.URL + "/abc")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "id=root") {
		t.Errorf("unknown path did not fall back to index.html: %q", body)
	}

	asset, err := srv.Client().Get(srv.URL + "/assets/app.123.js")
	if err != nil {
		t.Fatal(err)
	}
	defer asset.Body.Close()
	ab, _ := io.ReadAll(asset.Body)
	if string(ab) != "console.log(1)" {
		t.Errorf("asset body = %q", ab)
	}
	if cc := asset.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("fingerprinted asset Cache-Control = %q, want immutable", cc)
	}
}

func TestConfigEndpointMirrorsServerLimits(t *testing.T) {
	srv := newTestServer(t)

	resp, err := srv.Client().Get(srv.URL + "/api/config")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var cfg map[string]any
	json.NewDecoder(resp.Body).Decode(&cfg)
	if cfg["maxChars"] != float64(4000) {
		t.Errorf("maxChars = %v, want 4000", cfg["maxChars"])
	}
	if cfg["retentionDays"] != float64(30) {
		t.Errorf("retentionDays = %v, want 30", cfg["retentionDays"])
	}
}

// Codes are issued from a permutation the server controls, so no paste can ever
// occupy a path the server itself needs.
func TestReservedCodesAreNeverIssued(t *testing.T) {
	srv := newTestServer(t)

	reserved := make(map[string]struct{}, len(ReservedCodes))
	for _, r := range ReservedCodes {
		reserved[r] = struct{}{}
	}
	for i := 0; i < 300; i++ {
		_, created := postJSON(t, srv, `{"content":"x"}`)
		if _, bad := reserved[strings.ToLower(created.Key)]; bad {
			t.Fatalf("issued a reserved code: %q", created.Key)
		}
	}
}
