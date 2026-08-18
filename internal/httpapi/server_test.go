package httpapi

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/YuYu1015/haste-server/internal/compress"
	"github.com/YuYu1015/haste-server/internal/config"
	"github.com/YuYu1015/haste-server/internal/id"
	"github.com/YuYu1015/haste-server/internal/store"
)

// maxCharsForTest is the fixture's character limit. Smaller than the shipped
// default so the suite stays fast; the behaviour under test scales with it.
const maxCharsForTest = 4000

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	codec, err := compress.New(compress.DefaultLevel)
	if err != nil {
		t.Fatalf("compress.New: %v", err)
	}
	t.Cleanup(codec.Close)

	cfg := &config.Config{
		MaxChars:    maxCharsForTest,
		ZstdLevel:   compress.DefaultLevel,
		MaxBytes:    16 << 20,
		CORSOrigins: []string{"*"},
		// Rate limiting has its own test; leave it off elsewhere so a burst of
		// table-driven cases cannot trip it.
		RateRPS: 0,
		// The lifetime bounds published by /api/config come from here.
		CleanupInterval: time.Hour,
	}

	st, err := store.Open(context.Background(), store.Options{
		Path:     filepath.Join(t.TempDir(), "haste.db"),
		CacheMB:  8,
		ReadPool: 4,
		MaxChars: cfg.MaxChars,
		MaxBytes: cfg.MaxBytes,
		Codec:    codec,
		IDs:      id.NewGenerator([]byte("test-secret"), id.DefaultMinLen, ReservedCodes),
	})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	// Mirrors the real shell closely enough to exercise the parts the server
	// touches: the markers it rewrites between, and a default title to fall
	// back to.
	ui := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(
			"<!doctype html><html><head><!--head:start-->\n" +
				"<title>haste</title>\n" +
				"<!--head:end--></head><body><div id=root></div></body></html>")},
		"assets/app.123.js": &fstest.MapFile{Data: []byte("console.log(1)")},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	srv := httptest.NewServer(New(cfg, st, ui, log).Handler())
	t.Cleanup(srv.Close)
	return srv
}

// createBody builds the request envelope through the encoder, because content
// with newlines in it cannot be pasted into a JSON literal by hand.
func createBody(t *testing.T, content, language string) string {
	t.Helper()
	body, err := json.Marshal(createRequest{Content: content, Language: language})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
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

func fetchRaw(t *testing.T, srv *httptest.Server, key string) string {
	t.Helper()
	resp, err := srv.Client().Get(srv.URL + "/raw/" + key)
	if err != nil {
		t.Fatalf("GET /raw/%s: %v", key, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /raw/%s = %d, want 200", key, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
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
	// No expiry is published: the server cannot promise one.
	if strings.Contains(created.URL, "expires") {
		t.Error("unexpected expiry in response")
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
	// Nothing about retention may reach the client, which must not display a
	// lifetime the server has no way to honour.
	for _, forbidden := range []string{"retention", "retentionDays", "expires", "ttl"} {
		if _, present := cfg[forbidden]; present {
			t.Errorf("/api/config leaked %q: %v", forbidden, cfg[forbidden])
		}
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

// A paste never changes, so a client that already has one should be able to ask
// for it again without paying for the body twice.
func TestConditionalGetReturnsNotModified(t *testing.T) {
	srv := newTestServer(t)
	_, created := postJSON(t, srv, createBody(t, strings.Repeat("log line\n", 400), "log"))

	for _, path := range []string{
		"/api/pastes/" + created.Key,
		"/raw/" + created.Key,
		"/download/" + created.Key,
		"/api/config",
	} {
		t.Run(path, func(t *testing.T) {
			first, err := srv.Client().Get(srv.URL + path)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(first.Body)
			first.Body.Close()

			tag := first.Header.Get("Etag")
			if tag == "" {
				t.Fatal("no ETag on the first response")
			}
			if len(body) == 0 {
				t.Fatal("first response had no body")
			}

			req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
			req.Header.Set("If-None-Match", tag)
			second, err := srv.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer second.Body.Close()

			if second.StatusCode != http.StatusNotModified {
				t.Errorf("status = %d, want 304", second.StatusCode)
			}
			if again, _ := io.ReadAll(second.Body); len(again) != 0 {
				t.Errorf("304 carried %d bytes of body", len(again))
			}
		})
	}
}

func TestGzip(t *testing.T) {
	srv := newTestServer(t)
	content := strings.Repeat("2024-06-01 INFO request completed status=200\n", 80)
	_, created := postJSON(t, srv, createBody(t, content, "log"))

	get := func(path string, acceptGzip bool) (*http.Response, []byte) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		// The stdlib strips the header and decodes transparently unless the
		// request sets it explicitly, which would hide what went over the wire.
		if acceptGzip {
			req.Header.Set("Accept-Encoding", "gzip")
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp, body
	}

	plain, plainBody := get("/raw/"+created.Key, false)
	if enc := plain.Header.Get("Content-Encoding"); enc != "" {
		t.Errorf("unrequested Content-Encoding = %q", enc)
	}
	if string(plainBody) != content {
		t.Fatal("uncompressed body does not match the paste")
	}

	zipped, zippedBody := get("/raw/"+created.Key, true)
	if enc := zipped.Header.Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", enc)
	}
	if !strings.Contains(zipped.Header.Get("Vary"), "Accept-Encoding") {
		t.Error("a compressed response must Vary on Accept-Encoding")
	}
	// Compressing changes the bytes, so the validator has to become weak.
	if tag := zipped.Header.Get("Etag"); !strings.HasPrefix(tag, `W/"`) {
		t.Errorf("Etag = %q, want a weak tag on the compressed representation", tag)
	}
	if len(zippedBody) >= len(plainBody) {
		t.Errorf("gzip produced %d bytes for a %d byte body", len(zippedBody), len(plainBody))
	}

	reader, err := gzip.NewReader(bytes.NewReader(zippedBody))
	if err != nil {
		t.Fatalf("response is not valid gzip: %v", err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != content {
		t.Error("decompressed body does not match the paste")
	}
	t.Logf("%d B -> %d B (%.1fx)", len(plainBody), len(zippedBody),
		float64(len(plainBody))/float64(len(zippedBody)))
}

// Below a few hundred bytes a gzip frame's own header makes the response
// bigger, so the small ones are left alone.
func TestSmallResponsesAreNotCompressed(t *testing.T) {
	srv := newTestServer(t)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/config", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if enc := resp.Header.Get("Content-Encoding"); enc != "" {
		t.Errorf("Content-Encoding = %q on a tiny response", enc)
	}
	if tag := resp.Header.Get("Etag"); !strings.HasPrefix(tag, `"`) {
		t.Errorf("Etag = %q, want a strong tag on an uncompressed response", tag)
	}
}

// The frontend is embedded, so it has no modification time for a cache to ask
// about; a content hash is what makes revalidation cheap instead of a re-download.
func TestStaticAssetsCarryETags(t *testing.T) {
	srv := newTestServer(t)

	for _, path := range []string{"/assets/app.123.js", "/"} {
		resp, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		tag := resp.Header.Get("Etag")
		resp.Body.Close()
		if tag == "" {
			t.Fatalf("%s has no ETag", path)
		}

		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		req.Header.Set("If-None-Match", tag)
		second, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(second.Body)
		second.Body.Close()

		if second.StatusCode != http.StatusNotModified {
			t.Errorf("%s: status = %d, want 304", path, second.StatusCode)
		}
		if len(body) != 0 {
			t.Errorf("%s: 304 carried %d bytes", path, len(body))
		}
	}
}

// A missing file must say so. Answering it with the SPA shell returns 200 and
// an HTML body, which a browser then fails to parse as script or refuses to
// use as an icon — with nothing in the response to explain why.
func TestMissingStaticFileIs404(t *testing.T) {
	srv := newTestServer(t)

	for _, path := range []string{"/favicon.ico", "/assets/gone.js", "/nope.png", "/manifest.json"} {
		resp, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, resp.StatusCode)
		}
	}

	// A share code has no extension, so it still reaches the client router.
	resp, err := srv.Client().Get(srv.URL + "/aB3xK9pQ")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "id=root") {
		t.Errorf("a code-shaped path should still serve the shell: status %d", resp.StatusCode)
	}
}

// A shared link should say what is behind it, so the shell carries the paste's
// own title and description rather than the site's generic ones.
func TestPastePageCarriesItsOwnMetadata(t *testing.T) {
	srv := newTestServer(t)
	content := strings.Repeat("final item = compute();\n", 30)
	_, created := postJSON(t, srv, createBody(t, content, "dart"))

	fetch := func(path string) string {
		t.Helper()
		resp, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return string(body)
	}

	page := fetch("/" + created.Key)
	chars := strconv.Itoa(created.Chars)
	for _, want := range []string{"Dart", chars, created.Key} {
		if !strings.Contains(page, want) {
			t.Errorf("paste page metadata is missing %q", want)
		}
	}

	// The content itself must never reach the head: an unfurl is fetched and
	// cached by a third party, and pastes hold logs and configuration.
	if strings.Contains(page, "compute()") {
		t.Error("paste content leaked into the shell")
	}

	// The landing page keeps the generic description.
	if home := fetch("/"); strings.Contains(home, "Dart") || strings.Contains(home, chars+" 字元") {
		t.Error("the landing page picked up a paste's metadata")
	}

	// An unknown code falls back rather than inventing anything.
	if missing := fetch("/zzzzzzzz"); !strings.Contains(missing, "<title>haste</title>") {
		t.Error("an unknown code should fall back to the default head")
	}
}

// Two pastes share the URL shape but not the document, so a validator computed
// over the shell alone would let a cache serve one preview for the other.
func TestPastePagesHaveDistinctETags(t *testing.T) {
	srv := newTestServer(t)
	_, first := postJSON(t, srv, createBody(t, "package main", "go"))
	_, second := postJSON(t, srv, createBody(t, strings.Repeat("x", 500), "python"))

	tag := func(path string) string {
		t.Helper()
		resp, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.Header.Get("Etag")
	}

	a, b, home := tag("/"+first.Key), tag("/"+second.Key), tag("/")
	if a == "" || b == "" || home == "" {
		t.Fatal("a shell response carried no ETag")
	}
	if a == b {
		t.Error("two different pastes produced the same ETag")
	}
	if a == home || b == home {
		t.Error("a paste page shares the landing page's ETag")
	}
}

// Crawlable so a shared link can be unfurled, never listed in search results.
func TestPasteResponsesAreNoIndex(t *testing.T) {
	srv := newTestServer(t)
	_, created := postJSON(t, srv, createBody(t, "package main", "go"))

	for _, path := range []string{
		"/" + created.Key,
		"/api/pastes/" + created.Key,
		"/raw/" + created.Key,
		"/download/" + created.Key,
		"/documents/" + created.Key,
	} {
		resp, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if tag := resp.Header.Get("X-Robots-Tag"); !strings.Contains(tag, "noindex") {
			t.Errorf("%s: X-Robots-Tag = %q, want noindex", path, tag)
		}
	}

	// The landing page is the one thing worth finding in a search.
	resp, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if tag := resp.Header.Get("X-Robots-Tag"); tag != "" {
		t.Errorf("the landing page should stay indexable, got %q", tag)
	}
}

// TestCreateAcceptsEscapedJSONAtLimit guards the request body cap against being
// derived from raw UTF-8 alone.
//
// MaxChars counts code points, but a client chooses how many bytes each one
// costs on the wire: JSON permits \uXXXX for any character, and Python's
// json.dumps emits it by default. A CJK character then costs 6 bytes, and one
// outside the BMP is written as a surrogate pair costing 12 for what the server
// counts as a single character. A cap sized at 4 bytes per character rejects
// both with 413 before anything has counted a single character.
func TestCreateAcceptsEscapedJSONAtLimit(t *testing.T) {
	srv := newTestServer(t)

	// Written by hand rather than through json.Marshal, which leaves non-ASCII
	// as raw UTF-8 and so cannot produce the encoding under test.
	escape := func(r rune, count int) string {
		var b strings.Builder
		b.WriteString(`{"content":"`)
		for i := 0; i < count; i++ {
			if r > 0xFFFF {
				hi, lo := utf16.EncodeRune(r)
				fmt.Fprintf(&b, `\u%04x\u%04x`, hi, lo)
				continue
			}
			fmt.Fprintf(&b, `\u%04x`, r)
		}
		b.WriteString(`"}`)
		return b.String()
	}

	for _, tc := range []struct {
		name string
		char rune
	}{
		{"cjk", '測'},             // 6 bytes escaped
		{"astral", '\U0001F600'}, // 12 bytes escaped, still one code point
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := escape(tc.char, maxCharsForTest)
			resp, out := postJSON(t, srv, body)
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("status = %d for a %d byte body of %d characters, want 201",
					resp.StatusCode, len(body), maxCharsForTest)
			}

			// And the round trip preserves every character, so widening the cap
			// did not quietly let a truncated body through.
			got := fetchRaw(t, srv, out.Key)
			if n := utf8.RuneCountInString(got); n != maxCharsForTest {
				t.Fatalf("round trip returned %d characters, want %d", n, maxCharsForTest)
			}
		})
	}
}

func TestCreateWithALifetime(t *testing.T) {
	srv := newTestServer(t)

	t.Run("json envelope", func(t *testing.T) {
		body, err := json.Marshal(map[string]any{"content": "temporary", "expiresIn": 3600})
		if err != nil {
			t.Fatal(err)
		}
		resp, out := postJSON(t, srv, string(body))
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want 201", resp.StatusCode)
		}
		if out.ExpiresAt == nil {
			t.Fatal("expiresAt is absent from the response")
		}
		if d := time.Until(*out.ExpiresAt); d < 59*time.Minute || d > time.Hour+time.Minute {
			t.Errorf("expiresAt is %s away, want about an hour", d)
		}
	})

	t.Run("raw body with a duration", func(t *testing.T) {
		// The raw path has no envelope, so curl passes the lifetime in the query
		// string — as a duration here, which is what a person types.
		resp, err := srv.Client().Post(srv.URL+"/api/pastes?expiresIn=2h", "text/plain",
			strings.NewReader("temporary"))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		var out pasteResponse
		json.NewDecoder(resp.Body).Decode(&out)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want 201", resp.StatusCode)
		}
		if out.ExpiresAt == nil {
			t.Fatal("expiresAt is absent from the response")
		}
		if d := time.Until(*out.ExpiresAt); d < 119*time.Minute || d > 2*time.Hour+time.Minute {
			t.Errorf("expiresAt is %s away, want about two hours", d)
		}
	})

	// Omission has to stay omission: a null expiresAt would read as a promise
	// that the paste is permanent, which nothing here can make.
	t.Run("no lifetime asked for", func(t *testing.T) {
		resp, err := srv.Client().Post(srv.URL+"/api/pastes", "application/json",
			strings.NewReader(createBody(t, "permanent", "")))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		var raw map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			t.Fatal(err)
		}
		if _, present := raw["expiresAt"]; present {
			t.Errorf("expiresAt is present as %v for a paste that asked for no lifetime", raw["expiresAt"])
		}
	})
}

func TestCreateRejectsBadLifetimes(t *testing.T) {
	srv := newTestServer(t)

	for _, tc := range []struct{ name, body string }{
		{"under the minimum", `{"content":"x","expiresIn":60}`},
		{"over the maximum", `{"content":"x","expiresIn":31536000}`},
		{"negative", `{"content":"x","expiresIn":-3600}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := postJSON(t, srv, tc.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
		})
	}

	t.Run("unparseable in the query string", func(t *testing.T) {
		resp, err := srv.Client().Post(srv.URL+"/api/pastes?expiresIn=soon", "text/plain",
			strings.NewReader("x"))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", resp.StatusCode)
		}
	})
}

// A shared cache must not outlive the paste it is holding, or the link keeps
// working at the edge after the server has retired it.
func TestCacheControlIsTrimmedToTheRemainingLifetime(t *testing.T) {
	srv := newTestServer(t)

	body, err := json.Marshal(map[string]any{"content": "temporary", "expiresIn": 3600})
	if err != nil {
		t.Fatal(err)
	}
	_, short := postJSON(t, srv, string(body))
	_, long := postJSON(t, srv, createBody(t, "permanent", ""))

	for _, tc := range []struct{ name, path, want string }{
		{"paste outliving the cache window", "/api/pastes/" + long.Key, "max-age=300"},
		{"raw of the same", "/raw/" + long.Key, "max-age=300"},
		{"paste with an hour left", "/api/pastes/" + short.Key, "max-age=300"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := srv.Client().Get(srv.URL + tc.path)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if got := resp.Header.Get("Cache-Control"); !strings.Contains(got, tc.want) {
				t.Errorf("Cache-Control = %q, want it to contain %q", got, tc.want)
			}
		})
	}

	// The interesting case is a paste with less left than the cache window,
	// which Create cannot produce because the minimum lifetime is an hour.
	t.Run("forty seconds left", func(t *testing.T) {
		rec := httptest.NewRecorder()
		setCacheControl(rec, &store.Paste{ExpiresAt: time.Now().Add(40 * time.Second)})

		got := rec.Header().Get("Cache-Control")
		age, err := strconv.Atoi(strings.TrimSuffix(
			strings.TrimPrefix(got, "public, max-age="), ", immutable"))
		if err != nil {
			t.Fatalf("Cache-Control = %q, want a public max-age", got)
		}
		// Never above the remaining lifetime — a cache that outlives the paste
		// is the bug this exists to prevent — and never so far below it that
		// the trimming has stopped being a trim.
		if age > 40 || age < 38 {
			t.Errorf("max-age = %d, want 38..40 for a paste with forty seconds left", age)
		}
	})

	t.Run("already gone", func(t *testing.T) {
		rec := httptest.NewRecorder()
		setCacheControl(rec, &store.Paste{ExpiresAt: time.Now().Add(-time.Second)})
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("Cache-Control = %q, want no-store", got)
		}
	})
}

func TestConfigPublishesTheLifetimeBounds(t *testing.T) {
	srv := newTestServer(t)

	resp, err := srv.Client().Get(srv.URL + "/api/config")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var cfg map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		t.Fatal(err)
	}
	// The picker's ladder and the "deletion can lag by one cleanup" note are
	// both built from these, so a client that hard-codes them would drift.
	for key, want := range map[string]float64{
		"minExpirySecs":    store.MinTTL.Seconds(),
		"maxExpirySecs":    store.MaxTTL.Seconds(),
		"cleanupEverySecs": time.Hour.Seconds(),
	} {
		if got, ok := cfg[key].(float64); !ok || got != want {
			t.Errorf("%s = %v, want %v", key, cfg[key], want)
		}
	}
}
