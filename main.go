package main

import (
	"github.com/gozeloglu/rel/cmd"
)

// Injected at build time by GoReleaser via -ldflags.
var (
	version = ""
	commit  = ""
	date    = ""
	builtBy = ""
)

func main() {
	cmd.SetVersionInfo(version, commit, date, builtBy)
	cmd.Execute()
}
