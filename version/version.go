// Package version provides the build-time version injected at compile time.
package version

import "runtime/debug"

// Version is set at build time via -ldflags. Defaults to "dev" for local builds.
var Version = "dev"

// Commit returns the VCS commit hash from debug.ReadBuildInfo if available,
// falling back to "unknown".
func Commit() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			rev := setting.Value
			if len(rev) > 8 {
				rev = rev[:8]
			}
			return rev
		}
	}
	return "unknown"
}
