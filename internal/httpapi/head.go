package httpapi

import (
	"bytes"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/YuYu1015/haste-server/internal/store"
)

// The head between these markers is rewritten per request. Everything outside
// them — icons, theme colours, the module script — is the same on every page and
// is written straight through.
const (
	headStart = "<!--head:start-->"
	headEnd   = "<!--head:end-->"
)

// shell is index.html split at those markers, so a response is three writes
// rather than a search-and-replace over the document on every request.
type shell struct {
	before      []byte
	defaultHead []byte
	after       []byte
}

// newShell splits the built index.html. A document without the markers is kept
// whole and simply never gets per-paste metadata: losing a link preview is a
// better failure than serving a mangled page.
func newShell(index []byte) shell {
	start := bytes.Index(index, []byte(headStart))
	end := bytes.Index(index, []byte(headEnd))
	if start < 0 || end < 0 || end < start {
		return shell{before: index}
	}
	return shell{
		before:      index[:start+len(headStart)],
		defaultHead: index[start+len(headStart) : end],
		after:       index[end:],
	}
}

// render returns the document for a paste, or the untouched default when there
// is none.
func (s shell) render(p *store.Paste) []byte {
	if p == nil || s.after == nil {
		return s.document(s.defaultHead)
	}
	return s.document([]byte(pasteHead(p)))
}

func (s shell) document(head []byte) []byte {
	out := make([]byte, 0, len(s.before)+len(head)+len(s.after))
	out = append(out, s.before...)
	out = append(out, head...)
	return append(out, s.after...)
}

// pasteHead describes a paste to whatever is unfurling the link.
//
// Only metadata goes in: the language and the size say what is behind the link,
// which is what a reader wants to know before clicking. The content itself
// never appears — an unfurl is fetched and cached by a third party, and pastes
// routinely hold logs and configuration that should not end up there.
func pasteHead(p *store.Paste) string {
	language := languageLabel(p.Language)
	summary := fmt.Sprintf("%s · %s 字元", language, thousands(p.Chars))
	description := fmt.Sprintf("在 haste 檢視這則貼文（分享碼 %s）", p.Code)
	// A temporary paste says so in the preview, where it is most useful: the
	// person deciding whether to open the link later is exactly the person who
	// needs to know it will not be there.
	if left := remaining(p); left != "" {
		summary += " · " + left + "後刪除"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n    <title>%s · haste</title>\n", html.EscapeString(summary))
	for _, tag := range [][2]string{
		{`<meta name="description" content=%q />`, description},
		{`<meta property="og:title" content=%q />`, summary},
		{`<meta property="og:description" content=%q />`, description},
		{`<meta name="twitter:title" content=%q />`, summary},
		{`<meta name="twitter:description" content=%q />`, description},
	} {
		fmt.Fprintf(&b, "    ")
		fmt.Fprintf(&b, strings.Replace(tag[0], "%q", `"%s"`, 1), html.EscapeString(tag[1]))
		b.WriteString("\n")
	}
	b.WriteString("    ")
	return b.String()
}

// remaining renders how long a paste has left, rounded down to the unit a
// reader actually cares about. Empty when no lifetime was set, and when the
// paste is already gone — a preview is rendered from cached metadata and might
// outlive the paste, and "0 分鐘後刪除" is worse than saying nothing.
func remaining(p *store.Paste) string {
	if p.ExpiresAt.IsZero() {
		return ""
	}
	left := time.Until(p.ExpiresAt)
	switch {
	case left <= 0:
		return ""
	case left < time.Hour:
		return fmt.Sprintf("%d 分鐘", int(left.Minutes()))
	case left < 24*time.Hour:
		return fmt.Sprintf("%d 小時", int(left.Hours()))
	default:
		return fmt.Sprintf("%d 天", int(left.Hours()/24))
	}
}

// languageLabel turns a stored language id into something worth reading. The
// picker's own labels live in the frontend; this covers the handful that would
// otherwise read as an identifier.
func languageLabel(language string) string {
	if language == "" {
		return "純文字"
	}
	if label, ok := languageLabels[language]; ok {
		return label
	}
	// Everything else is already its own name once capitalised: go -> Go.
	return strings.ToUpper(language[:1]) + language[1:]
}

var languageLabels = map[string]string{
	"cpp":         "C++",
	"csharp":      "C#",
	"fsharp":      "F#",
	"objective-c": "Objective-C",
	"javascript":  "JavaScript",
	"typescript":  "TypeScript",
	"jsx":         "JSX",
	"tsx":         "TSX",
	"php":         "PHP",
	"sql":         "SQL",
	"html":        "HTML",
	"css":         "CSS",
	"scss":        "SCSS",
	"json":        "JSON",
	"jsonc":       "JSON",
	"json5":       "JSON5",
	"yaml":        "YAML",
	"toml":        "TOML",
	"ini":         "INI",
	"xml":         "XML",
	"csv":         "CSV",
	"graphql":     "GraphQL",
	"powershell":  "PowerShell",
	"docker":      "Dockerfile",
	"make":        "Makefile",
	"cmake":       "CMake",
	"latex":       "LaTeX",
	"ocaml":       "OCaml",
	"proto":       "Protobuf",
	"hcl":         "HCL",
	"http":        "HTTP",
	"wasm":        "WebAssembly",
	"dotenv":      "dotenv",
	"text":        "純文字",
	"log":         "Log",
	"diff":        "Diff",
}

// thousands groups a count the way the UI's own counter does.
func thousands(n int) string {
	digits := fmt.Sprintf("%d", n)
	if len(digits) <= 3 {
		return digits
	}

	var b strings.Builder
	lead := len(digits) % 3
	if lead > 0 {
		b.WriteString(digits[:lead])
	}
	for i := lead; i < len(digits); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}
