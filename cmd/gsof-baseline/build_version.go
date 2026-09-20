package main

import (
	"fmt"
	"runtime/debug"
)

// buildStamp is optionally set at link time (see build.sh) so each binary build is identifiable
// even when vcs.revision is unchanged (e.g. dirty working tree).
var buildStamp string

// buildDisplayVersion returns the semver in version.go plus a short VCS revision when
// the binary was built from a Git checkout (Go 1.18+ embeds vcs.revision with -buildvcs=true, the default).
func buildDisplayVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		if buildStamp != "" {
			return fmt.Sprintf("%s+%s", Version, buildStamp)
		}
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
		if buildStamp != "" {
			return fmt.Sprintf("%s+%s", Version, buildStamp)
		}
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
	if buildStamp != "" {
		suf += "." + buildStamp
	}
	return fmt.Sprintf("%s+%s", Version, suf)
}
