package web

import (
	"embed"
	"io/fs"
)

var (
	//go:embed static/index.html static/app.css static/app.js
	embeddedStaticFS embed.FS
	staticFS         fs.FS
	indexHTML        string
)

func init() {
	var err error
	staticFS, err = fs.Sub(embeddedStaticFS, "static")
	if err != nil {
		panic(err)
	}

	data, err := fs.ReadFile(staticFS, "index.html")
	if err != nil {
		panic(err)
	}
	indexHTML = string(data)
}
