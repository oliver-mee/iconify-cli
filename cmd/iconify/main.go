// Copyright 2026 Oliver Mee and contributors. Licensed under Apache-2.0. See LICENSE.

// Command iconify is the primary entrypoint for this CLI.
//
// The Printing Press names every generated binary <slug>-pp-cli, and
// cmd/iconify-pp-cli keeps that name so the CLI stays publishable to the public
// library unchanged. This is the same program under the name people actually
// type. Both build from the same internal/cli package, so they cannot drift.
package main

import (
	"os"

	"github.com/oliver-mee/iconify-cli/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}
