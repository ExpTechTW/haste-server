/**
 * The language catalogue offered in the picker, and the heuristics used to
 * guess one from the pasted text.
 *
 * Grammars are imported lazily and one file at a time, so the initial bundle
 * carries no TextMate grammars at all — only the one language a given paste
 * actually needs is ever fetched.
 */

export interface LanguageOption {
  id: string
  label: string
}

/** Shiki resolves these internally; no grammar has to be loaded. */
export const PLAIN = "text"

const loaders: Record<string, () => Promise<unknown>> = {
  bash: () => import("@shikijs/langs/bash"),
  c: () => import("@shikijs/langs/c"),
  cpp: () => import("@shikijs/langs/cpp"),
  csharp: () => import("@shikijs/langs/csharp"),
  css: () => import("@shikijs/langs/css"),
  diff: () => import("@shikijs/langs/diff"),
  docker: () => import("@shikijs/langs/docker"),
  go: () => import("@shikijs/langs/go"),
  graphql: () => import("@shikijs/langs/graphql"),
  html: () => import("@shikijs/langs/html"),
  ini: () => import("@shikijs/langs/ini"),
  java: () => import("@shikijs/langs/java"),
  javascript: () => import("@shikijs/langs/javascript"),
  json: () => import("@shikijs/langs/json"),
  kotlin: () => import("@shikijs/langs/kotlin"),
  lua: () => import("@shikijs/langs/lua"),
  markdown: () => import("@shikijs/langs/markdown"),
  nginx: () => import("@shikijs/langs/nginx"),
  php: () => import("@shikijs/langs/php"),
  powershell: () => import("@shikijs/langs/powershell"),
  python: () => import("@shikijs/langs/python"),
  ruby: () => import("@shikijs/langs/ruby"),
  rust: () => import("@shikijs/langs/rust"),
  sql: () => import("@shikijs/langs/sql"),
  swift: () => import("@shikijs/langs/swift"),
  toml: () => import("@shikijs/langs/toml"),
  tsx: () => import("@shikijs/langs/tsx"),
  typescript: () => import("@shikijs/langs/typescript"),
  xml: () => import("@shikijs/langs/xml"),
  yaml: () => import("@shikijs/langs/yaml"),
}

export const LANGUAGES: LanguageOption[] = [
  { id: PLAIN, label: "Plain text" },
  { id: "bash", label: "Bash" },
  { id: "c", label: "C" },
  { id: "cpp", label: "C++" },
  { id: "csharp", label: "C#" },
  { id: "css", label: "CSS" },
  { id: "diff", label: "Diff" },
  { id: "docker", label: "Dockerfile" },
  { id: "go", label: "Go" },
  { id: "graphql", label: "GraphQL" },
  { id: "html", label: "HTML" },
  { id: "ini", label: "INI" },
  { id: "java", label: "Java" },
  { id: "javascript", label: "JavaScript" },
  { id: "json", label: "JSON" },
  { id: "kotlin", label: "Kotlin" },
  { id: "lua", label: "Lua" },
  { id: "markdown", label: "Markdown" },
  { id: "nginx", label: "Nginx" },
  { id: "php", label: "PHP" },
  { id: "powershell", label: "PowerShell" },
  { id: "python", label: "Python" },
  { id: "ruby", label: "Ruby" },
  { id: "rust", label: "Rust" },
  { id: "sql", label: "SQL" },
  { id: "swift", label: "Swift" },
  { id: "toml", label: "TOML" },
  { id: "tsx", label: "TSX" },
  { id: "typescript", label: "TypeScript" },
  { id: "xml", label: "XML" },
  { id: "yaml", label: "YAML" },
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
 * Rules are evaluated in order and the first match wins, so the distinctive
 * markers (shebangs, prologues, sigils) come before the generic ones.
 */
const rules: Array<{ id: string; test: (text: string) => boolean }> = [
  { id: "bash", test: (t) => /^#!.*\b(?:ba|z|k)?sh\b/.test(t) },
  { id: "python", test: (t) => /^#!.*\bpython[\d.]*\b/.test(t) },
  { id: "php", test: (t) => /^\s*<\?php\b/.test(t) },
  { id: "xml", test: (t) => /^\s*<\?xml\b/.test(t) },
  { id: "html", test: (t) => /^\s*<!doctype html|^\s*<html[\s>]/i.test(t) },
  { id: "diff", test: (t) => /^diff --git |^@@ -\d+/m.test(t) },
  { id: "json", test: isJson },
  { id: "docker", test: (t) => /^FROM \S+/m.test(t) && /^(RUN|COPY|CMD|ENTRYPOINT|WORKDIR) /m.test(t) },
  { id: "go", test: (t) => /^package \w+/m.test(t) && /\bfunc\b/.test(t) },
  { id: "rust", test: (t) => /\bfn +\w+\s*[(<]/.test(t) && /\b(let mut|impl |use std::|-> +\w+ *\{)/.test(t) },
  { id: "java", test: (t) => /\b(public|private) +(static +)?(final +)?(class|void|String)\b/.test(t) },
  { id: "csharp", test: (t) => /^using +System\b/m.test(t) || /\bnamespace +[\w.]+\s*[{;]/.test(t) },
  { id: "kotlin", test: (t) => /\bfun +\w+\s*\(/.test(t) && /\b(val|var) +\w+/.test(t) },
  { id: "swift", test: (t) => /\bfunc +\w+\s*\(/.test(t) && /\b(import (Foundation|SwiftUI|UIKit)|let +\w+ *:)/.test(t) },
  { id: "cpp", test: (t) => /^#include *<\w+>/m.test(t) && /\b(std::|template *<|namespace )/.test(t) },
  { id: "c", test: (t) => /^#include *[<"]/m.test(t) },
  { id: "python", test: (t) => /^\s*(def|class) +\w+.*:\s*$/m.test(t) || /^\s*(from +[\w.]+ +import|import) +\w+/m.test(t) },
  { id: "ruby", test: (t) => /^\s*(def +\w+.*\n)/m.test(t) && /^\s*end\s*$/m.test(t) },
  { id: "sql", test: (t) => /^\s*(select|insert into|update|delete from|create (table|index)|alter table)\b/i.test(t) },
  { id: "nginx", test: (t) => /^\s*(server|http|location)\s+[^\n]*\{/m.test(t) && /\b(listen|proxy_pass|server_name)\b/.test(t) },
  { id: "powershell", test: (t) => /\$(PSVersionTable|PSScriptRoot)\b|\b(Write-Host|Get-ChildItem|Set-Location)\b/.test(t) },
  { id: "tsx", test: (t) => /\b(interface +\w+|: *(string|number|boolean)\b|as +\w+)/.test(t) && /<[A-Z]\w*[\s/>]/.test(t) },
  { id: "typescript", test: (t) => /\b(interface +\w+ *\{|type +\w+ *=|: *(string|number|boolean)[;,)\s]|enum +\w+)/.test(t) },
  { id: "javascript", test: (t) => /\b(function\b|=>|const +\w+ *=|require\(|module\.exports)/.test(t) },
  { id: "css", test: (t) => /[.#]?[\w-]+\s*\{[^}]*:[^}]*;/.test(t) && /\b(color|margin|padding|display|font-size)\s*:/.test(t) },
  { id: "markdown", test: (t) => /^#{1,6} +\S/m.test(t) || /^```/m.test(t) },
  { id: "toml", test: (t) => /^\[[\w.]+\]\s*$/m.test(t) && /^\w+ *= */m.test(t) },
  { id: "ini", test: (t) => /^\[[^\]]+\]\s*$/m.test(t) && /^[\w.]+ *= */m.test(t) },
  { id: "yaml", test: isYaml },
  { id: "graphql", test: (t) => /\b(query|mutation|fragment) +\w+|\btype +\w+ +\{/.test(t) },
  { id: "lua", test: (t) => /\blocal +\w+ *=|\bfunction +\w+\(.*\)\s*$/m.test(t) },
]

/**
 * Guesses a language for freshly typed content. Returns PLAIN when nothing
 * matches, which is the right answer for the log dumps this server mostly sees.
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
  const lines = text.split("\n").filter((l) => l.trim() && !l.trimStart().startsWith("#"))
  if (lines.length < 2) return false
  // "key: value" or "- item" on most lines, which is what separates YAML from
  // prose that happens to contain a colon.
  const yamlish = lines.filter((l) => /^\s*(-\s+\S|[\w.$-]+ *:( |$))/.test(l)).length
  return yamlish / lines.length > 0.7
}
