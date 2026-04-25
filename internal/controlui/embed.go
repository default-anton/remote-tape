package controlui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

func FS() fs.FS {
	control, err := fs.Sub(embedded, "dist/control")
	if err != nil {
		panic("control UI embedded filesystem invalid: " + err.Error())
	}
	return control
}

func Built() bool {
	_, err := fs.Stat(FS(), "index.control.html")
	return err == nil
}
