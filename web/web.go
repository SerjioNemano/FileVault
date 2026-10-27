package web

import (
	"embed"
	"io/fs"
)

//go:embed *.html *.js *.css img
var staticFiles embed.FS

// Static returns the embedded web client files.
func Static() fs.FS {
	return staticFiles
}
