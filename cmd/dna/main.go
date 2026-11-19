// Command dna is the DomainNameAPI command line interface.
//
// Install:
//
//	go install github.com/domainreseller/cli/cmd/dna@latest
//
// See https://github.com/domainreseller/cli for documentation.
package main

import (
	"context"
	"os"

	"github.com/domainreseller/cli/internal/cli"
)

// Set by the release build (-ldflags "-X main.version=...").
var (
	version = ""
	commit  = ""
	date    = ""
)

func main() {
	build := cli.ResolveBuildInfo(cli.BuildInfo{Version: version, Commit: commit, Date: date})
	app := cli.NewApp(build)
	os.Exit(app.Run(context.Background(), os.Args[1:]))
}
