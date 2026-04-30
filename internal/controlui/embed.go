package controlui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

func FS() fs.FS {
	return subFS("dist/control")
}

func Built() bool {
	_, err := fs.Stat(FS(), "index.control.html")
	return err == nil
}

func subFS(dir string) fs.FS {
	fileSystem, err := fs.Sub(embedded, dir)
	if err != nil {
		return emptyFS{}
	}
	return fileSystem
}

type emptyFS struct{}

func (emptyFS) Open(name string) (fs.File, error) {
	return nil, fs.ErrNotExist
}
