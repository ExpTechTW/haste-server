/**
 * The language catalogue offered in the picker, and the heuristics used to
 * guess one from the pasted text.
 *
 * Grammars are imported lazily and one file at a time, so the initial bundle
 * carries no TextMate grammars at all — only the one language a given paste
 * actually needs is ever fetched. Adding an entry here therefore costs nothing
 * at runtime, which is why the list is deliberately broad.
 */

export interface LanguageOption {
  id: string
  label: string
  /** Extra terms the picker's search should match, e.g. "golang" for Go. */
  aliases?: string
}

/** Shiki resolves this internally; no grammar has to be loaded. */
export const PLAIN = "text"

const loaders: Record<string, () => Promise<unknown>> = {
  apache: () => import("@shikijs/langs/apache"),
  asm: () => import("@shikijs/langs/asm"),
  astro: () => import("@shikijs/langs/astro"),
  awk: () => import("@shikijs/langs/awk"),
  bash: () => import("@shikijs/langs/bash"),
  c: () => import("@shikijs/langs/c"),
  clojure: () => import("@shikijs/langs/clojure"),
  cmake: () => import("@shikijs/langs/cmake"),
  cpp: () => import("@shikijs/langs/cpp"),
  crystal: () => import("@shikijs/langs/crystal"),
  csharp: () => import("@shikijs/langs/csharp"),
  css: () => import("@shikijs/langs/css"),
  csv: () => import("@shikijs/langs/csv"),
  d: () => import("@shikijs/langs/d"),
  dart: () => import("@shikijs/langs/dart"),
  diff: () => import("@shikijs/langs/diff"),
  docker: () => import("@shikijs/langs/docker"),
  dotenv: () => import("@shikijs/langs/dotenv"),
  elixir: () => import("@shikijs/langs/elixir"),
  elm: () => import("@shikijs/langs/elm"),
  erlang: () => import("@shikijs/langs/erlang"),
  fsharp: () => import("@shikijs/langs/fsharp"),
  gleam: () => import("@shikijs/langs/gleam"),
  go: () => import("@shikijs/langs/go"),
  graphql: () => import("@shikijs/langs/graphql"),
  groovy: () => import("@shikijs/langs/groovy"),
  haskell: () => import("@shikijs/langs/haskell"),
  hcl: () => import("@shikijs/langs/hcl"),
  html: () => import("@shikijs/langs/html"),
  http: () => import("@shikijs/langs/http"),
  ini: () => import("@shikijs/langs/ini"),
  java: () => import("@shikijs/langs/java"),
  javascript: () => import("@shikijs/langs/javascript"),
  json: () => import("@shikijs/langs/json"),
  json5: () => import("@shikijs/langs/json5"),
  jsonc: () => import("@shikijs/langs/jsonc"),
  jsx: () => import("@shikijs/langs/jsx"),
  julia: () => import("@shikijs/langs/julia"),
  kotlin: () => import("@shikijs/langs/kotlin"),
  latex: () => import("@shikijs/langs/latex"),
  less: () => import("@shikijs/langs/less"),
  lisp: () => import("@shikijs/langs/lisp"),
  log: () => import("@shikijs/langs/log"),
  lua: () => import("@shikijs/langs/lua"),
  make: () => import("@shikijs/langs/make"),
  markdown: () => import("@shikijs/langs/markdown"),
  nginx: () => import("@shikijs/langs/nginx"),
  nim: () => import("@shikijs/langs/nim"),
  "objective-c": () => import("@shikijs/langs/objective-c"),
  ocaml: () => import("@shikijs/langs/ocaml"),
  pascal: () => import("@shikijs/langs/pascal"),
  perl: () => import("@shikijs/langs/perl"),
  php: () => import("@shikijs/langs/php"),
  powershell: () => import("@shikijs/langs/powershell"),
  prisma: () => import("@shikijs/langs/prisma"),
  properties: () => import("@shikijs/langs/properties"),
  proto: () => import("@shikijs/langs/proto"),
  python: () => import("@shikijs/langs/python"),
  r: () => import("@shikijs/langs/r"),
  racket: () => import("@shikijs/langs/racket"),
  regex: () => import("@shikijs/langs/regex"),
  ruby: () => import("@shikijs/langs/ruby"),
  rust: () => import("@shikijs/langs/rust"),
  scala: () => import("@shikijs/langs/scala"),
  scheme: () => import("@shikijs/langs/scheme"),
  scss: () => import("@shikijs/langs/scss"),
  shellsession: () => import("@shikijs/langs/shellsession"),
  solidity: () => import("@shikijs/langs/solidity"),
  sql: () => import("@shikijs/langs/sql"),
  svelte: () => import("@shikijs/langs/svelte"),
  swift: () => import("@shikijs/langs/swift"),
  tcl: () => import("@shikijs/langs/tcl"),
  terraform: () => import("@shikijs/langs/terraform"),
  toml: () => import("@shikijs/langs/toml"),
  tsx: () => import("@shikijs/langs/tsx"),
  typescript: () => import("@shikijs/langs/typescript"),
  v: () => import("@shikijs/langs/v"),
  verilog: () => import("@shikijs/langs/verilog"),
  vim: () => import("@shikijs/langs/vim"),
  vue: () => import("@shikijs/langs/vue"),
  wasm: () => import("@shikijs/langs/wasm"),
  xml: () => import("@shikijs/langs/xml"),
  yaml: () => import("@shikijs/langs/yaml"),
  zig: () => import("@shikijs/langs/zig"),
}

export const LANGUAGES: LanguageOption[] = [
  { id: PLAIN, label: "Plain text", aliases: "txt plain none" },
  { id: "log", label: "Log", aliases: "logs syslog output" },
  { id: "diff", label: "Diff", aliases: "patch git" },

  { id: "apache", label: "Apache" },
  { id: "asm", label: "Assembly", aliases: "asm x86" },
  { id: "astro", label: "Astro" },
  { id: "awk", label: "AWK" },
  { id: "bash", label: "Bash", aliases: "sh shell zsh" },
  { id: "c", label: "C" },
  { id: "clojure", label: "Clojure", aliases: "clj" },
  { id: "cmake", label: "CMake" },
  { id: "cpp", label: "C++", aliases: "cplusplus cxx" },
  { id: "crystal", label: "Crystal" },
  { id: "csharp", label: "C#", aliases: "csharp dotnet" },
  { id: "css", label: "CSS" },
  { id: "csv", label: "CSV" },
  { id: "d", label: "D" },
  { id: "dart", label: "Dart", aliases: "flutter" },
  { id: "docker", label: "Dockerfile", aliases: "docker container" },
  { id: "dotenv", label: "dotenv", aliases: "env environment" },
  { id: "elixir", label: "Elixir", aliases: "ex phoenix" },
  { id: "elm", label: "Elm" },
  { id: "erlang", label: "Erlang", aliases: "erl" },
  { id: "fsharp", label: "F#", aliases: "fsharp dotnet" },
  { id: "gleam", label: "Gleam" },
  { id: "go", label: "Go", aliases: "golang" },
  { id: "graphql", label: "GraphQL", aliases: "gql" },
  { id: "groovy", label: "Groovy", aliases: "gradle" },
  { id: "haskell", label: "Haskell", aliases: "hs" },
  { id: "hcl", label: "HCL" },
  { id: "html", label: "HTML" },
  { id: "http", label: "HTTP", aliases: "request headers" },
  { id: "ini", label: "INI" },
  { id: "java", label: "Java" },
  { id: "javascript", label: "JavaScript", aliases: "js node" },
  { id: "json", label: "JSON" },
  { id: "json5", label: "JSON5" },
  { id: "jsonc", label: "JSON with comments", aliases: "jsonc" },
  { id: "jsx", label: "JSX", aliases: "react" },
  { id: "julia", label: "Julia" },
  { id: "kotlin", label: "Kotlin", aliases: "kt android" },
  { id: "latex", label: "LaTeX", aliases: "tex" },
  { id: "less", label: "Less" },
  { id: "lisp", label: "Lisp" },
  { id: "lua", label: "Lua" },
  { id: "make", label: "Makefile", aliases: "make" },
  { id: "markdown", label: "Markdown", aliases: "md" },
  { id: "nginx", label: "Nginx" },
  { id: "nim", label: "Nim" },
  { id: "objective-c", label: "Objective-C", aliases: "objc" },
  { id: "ocaml", label: "OCaml" },
  { id: "pascal", label: "Pascal", aliases: "delphi" },
  { id: "perl", label: "Perl", aliases: "pl" },
  { id: "php", label: "PHP" },
  { id: "powershell", label: "PowerShell", aliases: "ps1 pwsh" },
  { id: "prisma", label: "Prisma" },
  { id: "properties", label: "Properties" },
  { id: "proto", label: "Protobuf", aliases: "protobuf grpc" },
  { id: "python", label: "Python", aliases: "py" },
  { id: "r", label: "R" },
  { id: "racket", label: "Racket" },
  { id: "regex", label: "Regex", aliases: "regexp pattern" },
  { id: "ruby", label: "Ruby", aliases: "rb rails" },
  { id: "rust", label: "Rust", aliases: "rs cargo" },
  { id: "scala", label: "Scala" },
  { id: "scheme", label: "Scheme" },
  { id: "scss", label: "SCSS", aliases: "sass" },
  { id: "shellsession", label: "Shell session", aliases: "console terminal" },
  { id: "solidity", label: "Solidity", aliases: "sol ethereum" },
  { id: "sql", label: "SQL", aliases: "postgres mysql sqlite" },
  { id: "svelte", label: "Svelte" },
  { id: "swift", label: "Swift", aliases: "ios" },
  { id: "tcl", label: "Tcl" },
  { id: "terraform", label: "Terraform", aliases: "tf" },
  { id: "toml", label: "TOML" },
  { id: "tsx", label: "TSX", aliases: "react typescript" },
  { id: "typescript", label: "TypeScript", aliases: "ts" },
  { id: "v", label: "V" },
  { id: "verilog", label: "Verilog" },
  { id: "vim", label: "Vim script", aliases: "vimscript" },
  { id: "vue", label: "Vue" },
  { id: "wasm", label: "WebAssembly", aliases: "wasm wat" },
  { id: "xml", label: "XML" },
  { id: "yaml", label: "YAML", aliases: "yml" },
  { id: "zig", label: "Zig" },
]

const labels = new Map(LANGUAGES.map((l) => [l.id, l.label]))

export function languageLabel(id: string | undefined): string {
  if (!id) return labels.get(PLAIN)!
  return labels.get(id) ?? id
}

export function loaderFor(id: string): (() => Promise<unknown>) | undefined {
  return loaders[id]
}

/**
 * Dart shares almost all of its surface syntax with the C family, so it is
 * matched on tokens that only Dart has rather than on shape. Any one strong
 * marker settles it; otherwise two weak ones have to agree.
 */
const DART_STRONG = [
  /^\s*import\s+'(?:dart|package):/m,
  /@override\b/, // Java capitalises its annotation, so the case matters
  /\b(?:StatelessWidget|StatefulWidget|BuildContext|runApp|setState)\b/,
  // Most pasted Dart is Flutter, and these type names exist nowhere else.
  /\b(?:WidgetRef|ConsumerWidget|ChangeNotifier|StateNotifier|StateProvider)\b/,
  /\bextends\s+State</,
  /\brequired\s+this\.\w+/,
  /\bsuper\.key\b/,
  /\blate\s+(?:final|var)\b/,
  /\bWidget\s+build\s*\(/,
  /\)\s*async\s*\{/, // async follows the parameter list; JS and C# put it first
  /\b(?:double|int|String|bool|num|void)\s+get\s+\w+/, // Dart getter syntax
]

const DART_WEAK = [
  /\bvoid\s+main\s*\(\s*\)/, // Java's main always takes args
  /<\s*[A-Z]\w*\s*>\s*\[/, // typed list literal, <String>['a']
  /\bfor\s*\(\s*final\s+\w+\s+in\b/,
  /\.toList\(\)/,
  /\bfinal\s+\w+\s*=/,
  /\bprint\('/,
]

function isDart(text: string): boolean {
  if (DART_STRONG.some((re) => re.test(text))) return true
  return DART_WEAK.filter((re) => re.test(text)).length >= 2
}

/** Markup that reads as JSX, including the lowercase host elements. */
const JSX_MARKERS = /className=|<\/[A-Za-z][\w.]*>|<[A-Z]\w*[\s/>]/

/**
 * Type syntax that plain JavaScript cannot contain. The annotated return type
 * matters most in practice — `): Promise<User[]>` is how most pasted TypeScript
 * announces itself, long before any interface shows up.
 */
const TS_MARKERS =
  /\binterface\s+\w+\s*\{|\btype\s+\w+\s*=|\benum\s+\w+\s*\{|:\s*(?:string|number|boolean|void|any|unknown|never)\b|\)\s*:\s*[A-Za-z_$][\w$]*\s*(?:<|\[|\{|;|=>)/
/** Anything that means the surrounding text is a script, not a document. */
const SCRIPT_MARKERS = /\b(?:function|const|let|=>|import |export )/

/**
 * Rules are evaluated in order and the first match wins, so the distinctive
 * markers (shebangs, prologues, sigils) come before the generic ones.
 */
const rules: Array<{ id: string; test: (text: string) => boolean }> = [
  { id: "bash", test: (t) => /^#!.*\b(?:ba|z|k)?sh\b/.test(t) },
  { id: "python", test: (t) => /^#!.*\bpython[\d.]*\b/.test(t) },
  { id: "perl", test: (t) => /^#!.*\bperl\b/.test(t) },
  { id: "php", test: (t) => /^\s*<\?php\b/.test(t) },
  { id: "xml", test: (t) => /^\s*<\?xml\b/.test(t) },
  { id: "html", test: (t) => /^\s*<!doctype html|^\s*<html[\s>]/i.test(t) },
  { id: "diff", test: (t) => /^diff --git |^@@ -\d+/m.test(t) },
  { id: "json", test: isJson },
  // Logs are the single most-pasted thing here, and a timestamped line is an
  // unambiguous signal, so they are settled before any keyword-based rule can
  // trip over a word that happens to appear in a message.
  { id: "log", test: isLog },
  { id: "docker", test: (t) => /^FROM \S+/m.test(t) && /^(RUN|COPY|CMD|ENTRYPOINT|WORKDIR) /m.test(t) },
  { id: "http", test: (t) => /^(GET|POST|PUT|PATCH|DELETE|HEAD) \S+ HTTP\/\d|^HTTP\/\d(?:\.\d)? \d{3}/m.test(t) },

  // Dart before the C-family rules: Flutter code is full of braces, generics
  // and annotations that a looser Java or TypeScript rule would happily claim.
  { id: "dart", test: isDart },

  { id: "go", test: (t) => /^package \w+/m.test(t) && /\bfunc\b/.test(t) },
  { id: "rust", test: (t) => /\bfn +\w+\s*[(<]/.test(t) && /\b(let mut|impl |use std::|-> +\w+ *\{)/.test(t) },
  { id: "zig", test: (t) => /\b(const \w+ = @import\(|pub fn |try \w+)/.test(t) },
  { id: "elixir", test: (t) => /^\s*defmodule +[A-Z]/m.test(t) || /\bdef +\w+.*\bdo\b/.test(t) },
  { id: "erlang", test: (t) => /^-module\(|^-export\(/m.test(t) },
  { id: "haskell", test: (t) => /^\s*module +[A-Z][\w.]* +where/m.test(t) || /^\w+ +:: +.+->/m.test(t) },
  { id: "scala", test: (t) => /\b(object +\w+ +extends|def +\w+.*: +\w+ *=|case class +\w+)/.test(t) },
  { id: "solidity", test: (t) => /^\s*pragma solidity\b/m.test(t) || /\bcontract +\w+ *\{/.test(t) },
  { id: "swift", test: (t) => /\bfunc +\w+\s*\(/.test(t) && /\b(import (Foundation|SwiftUI|UIKit)|let +\w+ *:)/.test(t) },
  { id: "kotlin", test: (t) => /\bfun +\w+\s*\(/.test(t) && /\b(val|var) +\w+/.test(t) },
  // C# before Java: both declare `public class`, but only C# has `using
  // System` or a namespace block, so checking it first settles the overlap.
  { id: "csharp", test: (t) => /^using +System\b/m.test(t) || /\bnamespace +[\w.]+\s*[{;]/.test(t) },
  { id: "java", test: (t) => /\b(public|private) +(static +)?(final +)?(class|void|String)\b/.test(t) },
  { id: "objective-c", test: (t) => /^\s*#import +[<"]/m.test(t) || /\[\[\w+ alloc\] init/.test(t) },
  { id: "cpp", test: (t) => /^#include *<\w+>/m.test(t) && /\b(std::|template *<|namespace )/.test(t) },
  { id: "c", test: (t) => /^#include *[<"]/m.test(t) },
  { id: "python", test: (t) => /^\s*(def|class) +\w+.*:\s*$/m.test(t) || /^\s*(from +[\w.]+ +import|import) +\w+/m.test(t) },
  { id: "ruby", test: (t) => /^\s*(def +\w+.*\n)/m.test(t) && /^\s*end\s*$/m.test(t) },
  { id: "lua", test: (t) => /\blocal +\w+ *=/.test(t) && /\b(function|end)\b/.test(t) },
  { id: "perl", test: (t) => /\buse strict;|\bmy +[$@%]\w+/.test(t) },
  { id: "r", test: (t) => /<- *(function\(|c\()/.test(t) || /\blibrary\(\w+\)/.test(t) },
  { id: "julia", test: (t) => /^\s*using +[A-Z]\w*/m.test(t) && /\bfunction\b.*\bend\b/s.test(t) },
  { id: "sql", test: (t) => /^\s*(select|insert into|update|delete from|create (table|index)|alter table)\b/i.test(t) },
  { id: "prisma", test: (t) => /^\s*(model|generator|datasource) +\w+ *\{/m.test(t) },
  { id: "proto", test: (t) => /^\s*syntax *= *"proto[23]"/m.test(t) || /^\s*message +\w+ *\{/m.test(t) },
  // A brace must follow the operation name; otherwise a log line reading
  // "slow query duration=812ms" is enough to look like GraphQL.
  {
    id: "graphql",
    test: (t) =>
      /^\s*(query|mutation|subscription|fragment)\s+\w+[^\n]*\{/m.test(t) ||
      /\btype\s+\w+\s*(?:implements\s[\w\s&]+)?\{/.test(t),
  },
  { id: "terraform", test: (t) => /^\s*(resource|provider|variable|module) +"[^"]+"/m.test(t) },
  { id: "nginx", test: (t) => /^\s*(server|http|location)\s+[^\n]*\{/m.test(t) && /\b(listen|proxy_pass|server_name)\b/.test(t) },
  { id: "apache", test: (t) => /<VirtualHost|<Directory |^\s*(DocumentRoot|ServerName)\b/m.test(t) },
  { id: "make", test: (t) => /^[\w.%-]+:( |$).*\n\t\S/m.test(t) },
  { id: "cmake", test: (t) => /^\s*(cmake_minimum_required|project|add_executable|target_link_libraries)\s*\(/m.test(t) },
  { id: "powershell", test: (t) => /\$(PSVersionTable|PSScriptRoot)\b|\b(Write-Host|Get-ChildItem|Set-Location)\b/.test(t) },
  { id: "vue", test: (t) => /<template>[\s\S]*<\/template>/.test(t) },
  { id: "svelte", test: (t) => /<script[^>]*>[\s\S]*<\/script>/.test(t) && /\{#(if|each)\b/.test(t) },
  // Markup with no script around it is a document, not a component.
  {
    id: "html",
    test: (t) =>
      /<\/(?:div|span|p|body|head|table|ul|li|a|section|header|footer|h[1-6])>/.test(t) &&
      !SCRIPT_MARKERS.test(t),
  },
  { id: "tsx", test: (t) => TS_MARKERS.test(t) && JSX_MARKERS.test(t) },
  { id: "jsx", test: (t) => JSX_MARKERS.test(t) && SCRIPT_MARKERS.test(t) },
  { id: "typescript", test: (t) => TS_MARKERS.test(t) },
  { id: "javascript", test: (t) => /\b(function\b|=>|const +\w+ *=|require\(|module\.exports)/.test(t) },
  { id: "scss", test: (t) => /^\s*[@$][\w-]+/m.test(t) && /\{[^}]*:[^}]*;/.test(t) },
  { id: "css", test: (t) => /[.#]?[\w-]+\s*\{[^}]*:[^}]*;/.test(t) && /\b(color|margin|padding|display|font-size)\s*:/.test(t) },
  { id: "latex", test: (t) => /\\(documentclass|begin\{document\}|usepackage)\b/.test(t) },
  { id: "markdown", test: (t) => /^#{1,6} +\S/m.test(t) || /^```/m.test(t) },
  { id: "toml", test: (t) => /^\[[\w.]+\]\s*$/m.test(t) && /^\w+ *= */m.test(t) },
  { id: "ini", test: (t) => /^\[[^\]]+\]\s*$/m.test(t) && /^[\w.]+ *= */m.test(t) },
  { id: "dotenv", test: (t) => /^[A-Z][A-Z0-9_]* *=/m.test(t) && !/[{};]/.test(t) },
  { id: "yaml", test: isYaml },
  { id: "log", test: isLog },
  { id: "csv", test: isCsv },
]

/**
 * Guesses a language for freshly typed content. Returns PLAIN when nothing
 * matches, which is the right answer for arbitrary prose.
 */
export function detectLanguage(text: string): string {
  const sample = text.slice(0, 4000).trim()
  if (sample.length < 12) return PLAIN

  for (const rule of rules) {
    if (rule.test(sample)) return rule.id
  }
  return PLAIN
}

function isJson(text: string): boolean {
  const first = text[0]
  if (first !== "{" && first !== "[") return false
  try {
    JSON.parse(text)
    return true
  } catch {
    return false
  }
}

function isYaml(text: string): boolean {
  if (/[{};]\s*$/m.test(text)) return false
  const lines = contentLines(text)
  if (lines.length < 2) return false
  // "key: value" or "- item" on most lines, which is what separates YAML from
  // prose that happens to contain a colon.
  const yamlish = lines.filter((l) => /^\s*(-\s+\S|[\w.$-]+ *:( |$))/.test(l)).length
  return yamlish / lines.length > 0.7
}

function isLog(text: string): boolean {
  const lines = contentLines(text)
  if (lines.length < 2) return false
  // A leading timestamp or a bare severity word is what makes a log a log.
  const logish = lines.filter((l) =>
    /^\s*(\[?\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}|\d{2}:\d{2}:\d{2}|\[?(TRACE|DEBUG|INFO|WARN(ING)?|ERROR|FATAL)\b)/i.test(l),
  ).length
  return logish / lines.length > 0.6
}

function isCsv(text: string): boolean {
  const lines = contentLines(text)
  if (lines.length < 2) return false
  const counts = lines.slice(0, 20).map((l) => l.split(",").length)
  // Every row carrying the same number of commas, more than one, is the signal.
  return counts[0] > 1 && counts.every((c) => c === counts[0])
}

function contentLines(text: string): string[] {
  return text.split("\n").filter((l) => l.trim() && !l.trimStart().startsWith("#"))
}
