package main

// Version is semver-style (major.minor.patch) for gsof-dashboard. Bump it on every
// meaningful change so operators can confirm they are running a fresh build.
//
// Bump patch (e.g. 1.1.0 → 1.1.1) for localized fixes: one graph, one message type,
// copy, or small HTML/HTTP tweaks. Bump minor (e.g. 1.0.x → 1.1.0) when a release
// improves behavior across many GSOF / WGS-related messages (graphs, decoding surfaced
// in the UI, or stats that feed multiple subtype cards). Bump major only for breaking
// operator-visible behavior.
const Version = "1.2.1"
