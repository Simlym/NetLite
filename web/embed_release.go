//go:build webembed

package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var release embed.FS

func init() {
	var err error
	FS, err = fs.Sub(release, "dist")
	if err != nil {
		panic(err)
	}
}
