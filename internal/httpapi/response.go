package httpapi

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// gzipMinSize is the point below which compression stops paying for itself: a
// gzip frame carries about 20 bytes of header and trailer, and small JSON
// bodies routinely come out larger than they went in.
const gzipMinSize = 512

// gzipWriters pools encoders because a level-9 encoder allocates a sizeable
// window, and re-allocating one per response would cost more than the
// compression saves on bodies this small.
var gzipWriters = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(nil, gzip.BestCompression)
		return w
	},
}

// buffered gives a handler's response a content-derived ETag and, when the
// client asks for it, gzip.
//
// Both need the whole body before anything can be sent — the tag is a hash of
// it, and compressing in a stream would mean an unknown Content-Length — so the
// handler writes into memory first. That is only reasonable because every
// response on these routes is bounded: a paste is capped at a few thousand
// characters, and the JSON around it is smaller still.
func buffered(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec := &bufferedResponse{header: make(http.Header)}
		next(rec, r)

		out := w.Header()
		for key, values := range rec.header {
			out[key] = values
		}

		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		body := rec.body.Bytes()

		// Only a successful read has a representation worth naming. Errors are
		// small and change with the request, and a POST's response is not
		// something a client re-requests.
		taggable := status == http.StatusOK && (r.Method == http.MethodGet || r.Method == http.MethodHead)
		if taggable {
			tag := etagFor(body)
			out.Set("Etag", tag)
			if etagMatches(r.Header.Get("If-None-Match"), tag) {
				// A 304 carries no body, and therefore no body headers.
				out.Del("Content-Type")
				out.Del("Content-Length")
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}

		if encoded, ok := gzipped(r, out, body); ok {
			body = encoded
		}

		out.Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(status)
		if r.Method != http.MethodHead {
			w.Write(body)
		}
	}
}

// gzipped compresses the body when the client asked for it and the result is
// actually smaller. Behind the gateway this never fires: nginx sends
// `Accept-Encoding: ""` upstream and compresses itself, so the two never stack.
func gzipped(r *http.Request, header http.Header, body []byte) ([]byte, bool) {
	// The response varies by encoding whether or not this particular client
	// asked for one, so a shared cache has to key on it either way.
	header.Add("Vary", "Accept-Encoding")

	if len(body) < gzipMinSize ||
		!strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") ||
		header.Get("Content-Encoding") != "" ||
		!compressible(header.Get("Content-Type")) {
		return nil, false
	}

	writer := gzipWriters.Get().(*gzip.Writer)
	defer gzipWriters.Put(writer)

	var buf bytes.Buffer
	writer.Reset(&buf)
	if _, err := writer.Write(body); err != nil {
		return nil, false
	}
	if err := writer.Close(); err != nil {
		return nil, false
	}
	// Incompressible content — an already-compressed paste, say — comes out
	// larger; sending it would be worse than sending nothing at all.
	if buf.Len() >= len(body) {
		return nil, false
	}

	header.Set("Content-Encoding", "gzip")
	// The tag now names a representation whose bytes differ from the one it was
	// computed over, which is exactly what a weak validator is for.
	if tag := header.Get("Etag"); tag != "" && !strings.HasPrefix(tag, "W/") {
		header.Set("Etag", "W/"+tag)
	}
	return buf.Bytes(), true
}

func compressible(contentType string) bool {
	mediaType, _, _ := strings.Cut(contentType, ";")
	switch mediaType = strings.TrimSpace(mediaType); {
	case strings.HasPrefix(mediaType, "text/"),
		mediaType == "application/json",
		mediaType == "application/javascript",
		mediaType == "image/svg+xml":
		return true
	}
	return false
}

func etagFor(body []byte) string {
	sum := sha256.Sum256(body)
	// 12 bytes is far past the point where a collision is plausible, and keeps
	// the header short.
	return `"` + base64.RawURLEncoding.EncodeToString(sum[:12]) + `"`
}

// etagMatches compares weakly, which is the comparison a conditional GET calls
// for: a gzipped response carries W/"..." for the same content, and it should
// still count as unchanged.
func etagMatches(ifNoneMatch, tag string) bool {
	if ifNoneMatch == "" {
		return false
	}
	tag = strings.TrimPrefix(tag, "W/")
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == tag {
			return true
		}
	}
	return false
}

// bufferedResponse collects a handler's output in memory.
type bufferedResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (b *bufferedResponse) Header() http.Header { return b.header }

func (b *bufferedResponse) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}

func (b *bufferedResponse) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.body.Write(p)
}
