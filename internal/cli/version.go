package cli

import "runtime/debug"

// version is set at release build time via -ldflags "-X ...internal/cli.version=...".
var version = ""

func versionString() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(unknown)"
}
