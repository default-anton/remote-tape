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

func AuthFS() fs.FS {
	return subFS("dist/auth")
}

func JoinFS() fs.FS {
	return subFS("dist/join")
}

func Built() bool {
	_, controlErr := fs.Stat(FS(), "index.control.html")
	_, authErr := fs.Stat(AuthFS(), "index.auth.html")
	_, joinErr := fs.Stat(JoinFS(), "index.join.html")
	return controlErr == nil && authErr == nil && joinErr == nil
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
