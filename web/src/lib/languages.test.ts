import { describe, expect, it } from "vitest"

import { LANGUAGES, PLAIN, detectLanguage, loaderFor } from "./languages"

/**
 * Detection is a pile of heuristics, and heuristics regress silently: a rule
 * loosened for one language quietly steals another's pastes. Every case here
 * comes from a real misdetection, so the corpus only grows.
 */

describe("detectLanguage", () => {
  const cases: Array<[string, string, string]> = [
    [
      "dart",
      "flutter widget",
      `import 'package:flutter/material.dart';

class Greeting extends StatelessWidget {
  const Greeting({super.key, required this.name});
  final String name;

  @override
  Widget build(BuildContext context) => Text('Hello, $name');
}`,
    ],
    [
      "dart",
      "plain script with no flutter imports",
      `void main() {
  final items = <String>['a', 'b', 'c'];
  for (final item in items) {
    print('item: $item');
  }
}`,
    ],
    [
      // Used to lose to tsx: "as List" plus "<User>" reads as TypeScript generics.
      "dart",
      "async function returning a Future",
      `Future<List<User>> fetchUsers() async {
  final response = await http.get(Uri.parse('https://api.example.com/users'));
  return (jsonDecode(response.body) as List).map((e) => User.fromJson(e)).toList();
}`,
    ],
    [
      "dart",
      "stateful widget internals",
      `class _CounterState extends State<Counter> {
  int _count = 0;

  void _increment() {
    setState(() {
      _count++;
    });
  }
}`,
    ],
    [
      // Only marker is the lowercase @override and the getter syntax.
      "dart",
      "abstract class with getters",
      `abstract class Shape {
  double get area;
}

class Circle implements Shape {
  Circle(this.radius);
  final double radius;

  @override
  double get area => 3.14 * radius * radius;
}`,
    ],

    // Neighbours of Dart that must not be captured by its rules.
    [
      "java",
      "capitalised @Override annotation",
      `public class Service {
    @Override
    public String toString() {
        return "Service";
    }
}`,
    ],
    [
      "java",
      "Future without async",
      `public class Repo {
    private final Map<String, User> cache = new HashMap<>();

    public Future<User> findAsync(String id) {
        return executor.submit(() -> cache.get(id));
    }
}`,
    ],
    [
      // Used to lose to Java: both declare "public class".
      "csharp",
      "async Task with using System",
      `using System.Threading.Tasks;

namespace App {
    public class Client {
        public async Task<string> GetAsync(string url) {
            var response = await _http.GetAsync(url);
            return await response.Content.ReadAsStringAsync();
        }
    }
}`,
    ],
    [
      // Used to fall through to javascript: no interface, no type alias.
      "typescript",
      "annotated return type only",
      `export async function fetchUsers(): Promise<User[]> {
  const response = await fetch('/api/users')
  return (await response.json()) as User[]
}`,
    ],
    [
      // Used to lose to typescript: JSX detection only matched capitalised tags.
      "tsx",
      "component returning a host element",
      `export function Card({ title }: { title: string }) {
  return <div className="card">{title}</div>
}`,
    ],
    [
      "jsx",
      "component without type annotations",
      `export default function App() {
  const [n, setN] = useState(0)
  return <button onClick={() => setN(n + 1)}>{n}</button>
}`,
    ],
    [
      "html",
      "markup with no script around it",
      `<div class="card">
  <h1>Title</h1>
  <p>Some text</p>
</div>`,
    ],
    [
      // Used to lose to graphql: "slow query duration" matched its keyword rule.
      "log",
      "lines mentioning query, type and mutation",
      `2024-06-01 10:00:00 INFO  slow query duration=812ms rows=1043
2024-06-01 10:00:01 WARN  type mismatch on column id
2024-06-01 10:00:02 ERROR mutation failed for user 42`,
    ],
    [
      "graphql",
      "operation with a selection set",
      `query GetUser($id: ID!) {
  user(id: $id) {
    name
  }
}`,
    ],

    ["go", "package with func", 'package main\n\nimport "fmt"\n\nfunc main() {\n\tfmt.Println("hi")\n}'],
    ["python", "imports and def", 'import os\n\ndef main() -> None:\n    print(os.getcwd())'],
    ["rust", "fn with let mut", 'fn main() {\n    let mut total = 0;\n    println!("{}", total);\n}'],
    ["kotlin", "fun with val", 'fun main() {\n    val names = listOf("a")\n    names.forEach { println(it) }\n}'],
    ["swift", "Foundation import", 'import Foundation\n\nfunc greet(name: String) -> String {\n    let out = "hi"\n    return out\n}'],
    ["elixir", "defmodule", "defmodule Worker do\n  def init(state) do\n    {:ok, state}\n  end\nend"],
    ["ruby", "def and end", 'def greet(name)\n  puts "Hello"\nend'],
    ["php", "open tag", '<?php\nfunction greet(string $n): string {\n    return $n;\n}'],
    ["json", "object literal", '{"id": 1, "name": "test", "tags": ["a"]}'],
    ["yaml", "nested mapping", "services:\n  web:\n    image: nginx\n    ports:\n      - \"80:80\""],
    ["toml", "table header", '[package]\nname = "app"\nversion = "0.1.0"'],
    ["sql", "select statement", "SELECT id, name FROM users WHERE active = 1 ORDER BY id;"],
    ["bash", "shebang", '#!/usr/bin/env bash\nset -euo pipefail\nfor f in *.log; do gzip "$f"; done'],
    ["docker", "FROM and RUN", 'FROM node:22-alpine\nWORKDIR /app\nRUN npm ci\nCMD ["node", "x.js"]'],
    ["terraform", "resource block", 'resource "aws_s3_bucket" "assets" {\n  bucket = "my-assets"\n}'],
    ["diff", "git header", "diff --git a/main.go b/main.go\n@@ -1,3 +1,4 @@\n package main"],
    ["css", "rule with declarations", ".card {\n  display: flex;\n  color: #333;\n  padding: 1rem;\n}"],
    [
      "text",
      "ordinary prose",
      `Just some ordinary prose that happens to span
a couple of lines and mentions nothing technical at all.`,
    ],
  ]

  it.each(cases)("detects %s (%s)", (expected, _label, source) => {
    expect(detectLanguage(source)).toBe(expected)
  })

  it("returns plain text for content too short to judge", () => {
    expect(detectLanguage("hi")).toBe(PLAIN)
    expect(detectLanguage("")).toBe(PLAIN)
  })
})

describe("catalogue", () => {
  it("has a grammar loader for every entry except plain text", () => {
    const missing = LANGUAGES.filter((l) => l.id !== PLAIN && !loaderFor(l.id))
    expect(missing.map((l) => l.id)).toEqual([])
  })

  it("has no duplicate ids", () => {
    const ids = LANGUAGES.map((l) => l.id)
    expect(ids).toHaveLength(new Set(ids).size)
  })

  it("only ever detects languages the picker can offer", () => {
    // A detected id with no catalogue entry would highlight as something the
    // picker cannot show, and would download with the wrong extension.
    const offered = new Set(LANGUAGES.map((l) => l.id))
    for (const [expected] of [
      ["dart"],
      ["csharp"],
      ["tsx"],
      ["jsx"],
      ["log"],
      ["graphql"],
      ["html"],
      ["terraform"],
    ]) {
      expect(offered.has(expected)).toBe(true)
    }
  })
})
