// Package corpus generates representative pastes for measurement tests.
//
// The compression and storage measurements both need the same inputs, and both
// need them to be exactly the size the server actually accepts, so the
// generators live here rather than being copied into each package's tests. It
// is imported only from _test files and therefore never reaches the binary.
package corpus

import (
	"fmt"
	"math/rand"
	"strings"
)

// Chars is the size every sample is built to exactly.
//
// It is not the server's limit — that is ten times larger — but a representative
// paste, held constant so compression and storage numbers from different runs
// can be compared. Measuring at the limit instead would make the suites slow
// without changing what they show.
const Chars = 4000

// Kind names one flavour of paste.
type Kind struct {
	Name     string
	Generate func(seed int) string
}

// Kinds covers the range from "compresses beautifully" to "does not compress at
// all", including the CJK cases that cost three bytes per character.
var Kinds = []Kind{
	{"structured log", Log},
	{"json log lines", JSONLog},
	{"go source", Code},
	{"prose", Prose},
	{"cjk prose", CJK},
	{"incompressible", Random},
	{"incompressible cjk", RandomCJK},
}

// Mixed returns a corpus weighted the way a real instance would be: mostly logs
// and code, some prose, a little that will not compress.
func Mixed() [][]byte {
	var out [][]byte
	add := func(n int, gen func(int) string) {
		for i := 0; i < n; i++ {
			out = append(out, []byte(gen(i)))
		}
	}
	add(40, Log)
	add(40, JSONLog)
	add(40, Code)
	add(20, Prose)
	add(10, CJK)
	add(10, Random)
	return out
}

// pad trims or extends a sample to exactly Chars characters.
func pad(s string) string {
	r := []rune(s)
	if len(r) >= Chars {
		return string(r[:Chars])
	}
	return string(r) + strings.Repeat(" ", Chars-len(r))
}

func Log(seed int) string {
	var b strings.Builder
	for line := 0; b.Len() < 4*Chars; line++ {
		fmt.Fprintf(&b, "2024-06-%02dT10:%02d:%02d.117Z INFO  request completed method=GET path=/api/v1/users status=200 duration=%d.%02dms request_id=%08x\n",
			1+seed%28, line%60, (line*7)%60, line%400, line%100, seed*7919+line)
	}
	return pad(b.String())
}

func JSONLog(seed int) string {
	var b strings.Builder
	for line := 0; b.Len() < 4*Chars; line++ {
		fmt.Fprintf(&b, `{"level":"info","ts":"2024-06-01T10:%02d:%02d.117Z","caller":"server.go:%d","msg":"request completed","status":200,"request_id":"%08x"}`+"\n",
			line%60, (line*3)%60, 100+line%400, seed*104729+line)
	}
	return pad(b.String())
}

func Code(seed int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "package service%d\n\nimport (\n\t\"context\"\n\t\"fmt\"\n)\n\n", seed%9)
	for fn := 0; b.Len() < 4*Chars; fn++ {
		fmt.Fprintf(&b, "func (s *Service) Handle%d(ctx context.Context, id int64) (*Result, error) {\n\trow, err := s.db.QueryRowContext(ctx, query%d, id)\n\tif err != nil {\n\t\treturn nil, fmt.Errorf(\"handle: %%w\", err)\n\t}\n\treturn row, nil\n}\n\n", fn, fn)
	}
	return pad(b.String())
}

func Prose(seed int) string {
	words := strings.Fields(`the deployment should wait until the release window closes
		or proceed immediately given that the rollback plan has been tested twice
		this quarter and nobody has raised a concrete objection to shipping it now`)
	rng := rand.New(rand.NewSource(int64(seed)))
	var b strings.Builder
	for b.Len() < 4*Chars {
		b.WriteString(words[rng.Intn(len(words))])
		b.WriteByte(' ')
	}
	return pad(b.String())
}

func CJK(seed int) string {
	words := []rune("系統啟動失敗請檢查設定檔與資料庫連線狀態並重新嘗試操作紀錄已寫入日誌目錄")
	rng := rand.New(rand.NewSource(int64(seed)))
	b := make([]rune, Chars)
	for i := range b {
		b[i] = words[rng.Intn(len(words))]
	}
	return string(b)
}

func Random(seed int) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	rng := rand.New(rand.NewSource(int64(seed) + 1))
	b := make([]byte, Chars)
	for i := range b {
		b[i] = alphabet[rng.Intn(len(alphabet))]
	}
	return string(b)
}

// RandomCJK is the most expensive input the server will accept: three bytes per
// character, with no repetition for the compressor to exploit.
func RandomCJK(seed int) string {
	rng := rand.New(rand.NewSource(int64(seed) + 7))
	b := make([]rune, Chars)
	for i := range b {
		b[i] = rune(0x4E00 + rng.Intn(0x9FFF-0x4E00))
	}
	return string(b)
}
