package discobox_ui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:generate npm i
//go:generate npm run build
//go:embed all:build
var files embed.FS

func GetFileSystem() http.FileSystem {
	// Get the sub filesystem from the embedded files
	distFS, err := fs.Sub(files, "build")
	if err != nil {
		panic(err)
	}
	return http.FS(distFS)
}
