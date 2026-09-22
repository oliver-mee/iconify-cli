// Copyright 2026 Oliver Mee and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "runtime/debug"

// init fills in the version from the module build info when no ldflag set it.
// Release builds stamp version via goreleaser; `go install ...@v0.1.0` does not,
// but Go records the resolved module version, so use that instead of 0.0.0-dev.
func init() {
	if version != "0.0.0-dev" {
		return
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			version = v
		}
	}
}
