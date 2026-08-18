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

// Product is the name as it is written: capitalised, because it is a name.
const Product = "Haste"

// pasteHead describes a paste to whatever is unfurling the link.
//
// Only metadata goes in: what kind of thing it is and how big, which is what a
// reader wants to know before clicking. The content itself never appears — an
// unfurl is fetched and cached by a third party, and pastes routinely hold logs
// and configuration that should not end up there.
//
// A title, when one was given, replaces the generated summary rather than
// joining it: someone who bothered to name the paste has said what it is better
// than "Python · 410 字元" can.
func pasteHead(p *store.Paste) string {
	summary := fmt.Sprintf("%s · %s 字元", languageLabel(p.Language), thousands(p.Chars))
	description := "檢視這則 " + summary
	if p.Title != "" {
		summary = p.Title
	}
	// A temporary paste says so in the preview, where it is most useful: the
	// person deciding whether to open the link later is exactly the person who
	// needs to know it will not be there.
	if left := remaining(p); left != "" {
		description += "，" + left + "後刪除"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n    <title>%s · %s</title>\n", html.EscapeString(summary), Product)
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

// remaining renders how long a paste has left, in the largest unit that still
// says something.
//
// Rounded to nearest with the unit chosen afterwards, the same way the
// countdown in the client is: flooring makes a six-hour paste announce itself
// as "5 小時後刪除" the moment it is created, and a preview has no ticking clock
// to make that read as counting down.
//
// Empty when no lifetime was set, and when the paste is already gone — a
// preview is rendered from metadata a third party may have cached, and it can
// outlive the paste it describes.
func remaining(p *store.Paste) string {
	return remainingAt(p, time.Now())
}

// remainingAt is remaining with the clock passed in, so a test can sit exactly
// on a rounding boundary instead of a fraction of a millisecond below it.
func remainingAt(p *store.Paste, now time.Time) string {
	if p.ExpiresAt.IsZero() {
		return ""
	}
	left := p.ExpiresAt.Sub(now)
	if left <= 0 {
		return ""
	}

	if minutes := int(left.Round(time.Minute).Minutes()); minutes < 60 {
		return fmt.Sprintf("%d 分鐘", max(minutes, 1))
	}
	if hours := int(left.Round(time.Hour).Hours()); hours < 24 {
		return fmt.Sprintf("%d 小時", hours)
	}
	return fmt.Sprintf("%d 天", int((left+12*time.Hour).Hours()/24))
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
