package main

// Version is the semver base baked into the HTML banner. When the binary is built from Git,
// buildDisplayVersion() also appends a short revision (see cmd/gsof-dashboard/build_version.go).
//
// Bump patch (e.g. 1.1.0 → 1.1.1) for localized fixes: one graph, one message type,
// copy, or small HTML/HTTP tweaks. Bump minor (e.g. 1.0.x → 1.1.0) when a release
// improves behavior across many GSOF / WGS-related messages (graphs, decoding surfaced
// in the UI, or stats that feed multiple subtype cards). Bump major only for breaking
// operator-visible behavior.
const Version = "1.2.9"
