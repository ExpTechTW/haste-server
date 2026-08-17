// Package webui embeds the built frontend so the server ships as one binary.
//
// dist/ is produced by `npm run build` in web/ and copied here by the Makefile.
// A placeholder index.html is committed so `go build` works on a clean checkout
// before the frontend has ever been built.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

// FS returns the root of the built frontend.
func FS() (fs.FS, error) {
	return fs.Sub(embedded, "dist")
}
