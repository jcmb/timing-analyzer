package main

// Version is incremented whenever dashboard HTML, HTTP behavior, or related
// server logic changes so operators can confirm they are running a fresh build.
// Bump the patch (e.g. 1.0.4 → 1.0.5) when internal/gsof decoding changes, since
// the dashboard surfaces decoded GSOF fields. Bump for other meaningful changes too.
const Version = "1.0.83"
