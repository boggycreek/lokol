// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package version

import "fmt"

// Version is the current semantic version of lokol.
// In CI release builds, this is overridden via:
// -ldflags "-X github.com/boggycreek/lokol/liblokol/version.Version=v0.1.0"
var (
	Version   = "v0.1.0-alpha"
	GitCommit = "dev"
	BuildDate = "unknown"
)

// FullVersion returns formatted version string including commit and build date.
func FullVersion() string {
	return fmt.Sprintf("%s (commit: %s, built: %s)", Version, GitCommit, BuildDate)
}

