package main

import (
	"fmt"
	"runtime/debug"
)

// buildDisplayVersion returns the semver in version.go plus a short VCS revision when
// the binary was built from a Git checkout (Go 1.18+ embeds vcs.revision with -buildvcs=true, the default).
func buildDisplayVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Version
	}
	var rev, modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	if rev == "" {
		return Version
	}
	short := rev
	if len(short) > 9 {
		short = short[:9]
	}
	suf := short
	if modified == "true" {
		suf += ".dirty"
	}
	return fmt.Sprintf("%s+%s", Version, suf)
}
