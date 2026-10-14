package main

import (
	"embed"
	"io/fs"

	"github.com/raffleberry/cctv/cctv"
)

// The built UI lives in ui/dist (gitignored — see .gitignore). It must be
// built before the Go binary:
//
//	pnpm --dir ui install && pnpm --dir ui build && go build .
//
// A missing ui/dist fails `go build` here at compile time (unresolved embed
// pattern); cctv.SetWebFS panics at startup when index.html itself is absent.
//
//go:embed ui/dist
var embeddedDist embed.FS

func init() {
	sub, err := fs.Sub(embeddedDist, "ui/dist")
	if err != nil {
		panic("cctv: ui/dist not embedded (run `pnpm --dir ui build` first): " + err.Error())
	}
	cctv.SetWebFS(sub)
}
