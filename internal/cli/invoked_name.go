// Copyright 2026 Oliver Mee and contributors. Licensed under Apache-2.0. See LICENSE.

// This CLI ships two entrypoints for the same program: `iconify`, the name
// people type, and `iconify-pp-cli`, the Printing Press naming convention kept
// so the CLI stays publishable to the public library unchanged.
//
// The generated root command hardcodes the press name, so help and usage text
// would tell a caller who typed `iconify` to run `iconify-pp-cli` instead.
// Resolving the name from the actual invocation keeps every usage line, error,
// and example consistent with whichever entrypoint was used.

package cli

import (
	"os"
	"path/filepath"
	"strings"
)

// invokedName reports the binary name this process was started as, falling back
// to the press name when the invocation cannot be read.
func invokedName() string {
	if len(os.Args) == 0 {
		return "iconify-pp-cli"
	}
	base := filepath.Base(os.Args[0])
	base = strings.TrimSuffix(base, ".exe")
	// A go-run temp binary or a test harness name is not a real entrypoint.
	switch {
	case base == "", strings.HasPrefix(base, "."), strings.Contains(base, "go-build"),
		strings.HasSuffix(base, ".test"):
		return "iconify-pp-cli"
	}
	return base
}
