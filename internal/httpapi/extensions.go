package httpapi

// extensions maps a language hint onto the file extension a download should
// carry, so a saved paste opens in the right editor mode. Keys match the
// identifiers the frontend's language picker sends.
//
// Anything unlisted downloads as .txt, which is the correct answer for a log
// dump and a safe one for everything else.
var extensions = map[string]string{
	// shells and ops
	"bash":         "sh",
	"shellsession": "sh",
	"powershell":   "ps1",
	"awk":          "awk",
	"vim":          "vim",
	"make":         "mk",
	"cmake":        "cmake",
	"docker":       "dockerfile",
	"nginx":        "conf",
	"apache":       "conf",
	"terraform":    "tf",
	"hcl":          "hcl",
	"http":         "http",
	"log":          "log",

	// systems
	"c":           "c",
	"cpp":         "cpp",
	"csharp":      "cs",
	"objective-c": "m",
	"d":           "d",
	"go":          "go",
	"rust":        "rs",
	"zig":         "zig",
	"nim":         "nim",
	"crystal":     "cr",
	"v":           "v",
	"asm":         "asm",
	"verilog":     "v",
	"wasm":        "wat",
	"pascal":      "pas",

	// jvm and friends
	"java":    "java",
	"kotlin":  "kt",
	"groovy":  "groovy",
	"scala":   "scala",
	"clojure": "clj",

	// functional
	"elixir":  "ex",
	"erlang":  "erl",
	"haskell": "hs",
	"elm":     "elm",
	"gleam":   "gleam",
	"ocaml":   "ml",
	"fsharp":  "fs",
	"lisp":    "lisp",
	"scheme":  "scm",
	"racket":  "rkt",

	// application
	"dart":   "dart",
	"swift":  "swift",
	"python": "py",
	"ruby":   "rb",
	"perl":   "pl",
	"php":    "php",
	"lua":    "lua",
	"r":      "r",
	"julia":  "jl",
	"tcl":    "tcl",

	// web
	"javascript": "js",
	"jsx":        "jsx",
	"typescript": "ts",
	"tsx":        "tsx",
	"vue":        "vue",
	"svelte":     "svelte",
	"astro":      "astro",
	"html":       "html",
	"css":        "css",
	"less":       "less",
	"scss":       "scss",
	"xml":        "xml",

	// data and config
	"json":       "json",
	"jsonc":      "jsonc",
	"json5":      "json5",
	"yaml":       "yaml",
	"toml":       "toml",
	"ini":        "ini",
	"properties": "properties",
	"dotenv":     "env",
	"csv":        "csv",
	"sql":        "sql",
	"prisma":     "prisma",
	"graphql":    "graphql",
	"proto":      "proto",
	"solidity":   "sol",

	// prose and misc
	"markdown": "md",
	"latex":    "tex",
	"diff":     "diff",
}

// extensionFor returns the download extension for a language hint.
func extensionFor(language string) string {
	if ext, ok := extensions[language]; ok {
		return ext
	}
	return "txt"
}
