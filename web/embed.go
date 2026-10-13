// Package web embeds the release frontend. Development may use --web instead.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed index.html
var source embed.FS

// FS contains the built frontend when compiled with -tags webembed.
var FS fs.FS = source

func Handler() http.Handler { return http.FileServer(http.FS(FS)) }
