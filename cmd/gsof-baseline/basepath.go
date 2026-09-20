package main

import (
	"net/http"
	"strings"
)

func normalizeHTTPBasePath(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "/" {
		return ""
	}
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	return strings.TrimSuffix(s, "/")
}

func resolveHTTPBasePath(flagValues ...string) string {
	for _, v := range flagValues {
		if p := normalizeHTTPBasePath(v); p != "" {
			return p
		}
	}
	return ""
}

func publicPath(basePath, path string) string {
	if basePath == "" {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return basePath + path
}

// stripHTTPBasePath removes the configured public prefix from incoming paths.
// When the reverse proxy forwards the full URI (e.g. /baseline/api/…), the prefix is stripped.
// When the proxy already stripped the prefix but sends X-Forwarded-Prefix, paths pass through.
func stripHTTPBasePath(basePath string, next http.Handler) http.Handler {
	if basePath == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == basePath || strings.HasPrefix(path, basePath+"/") {
			path = strings.TrimPrefix(path, basePath)
			if path == "" {
				path = "/"
			}
			r.URL.Path = path
			next.ServeHTTP(w, r)
			return
		}
		if fwd := normalizeHTTPBasePath(r.Header.Get("X-Forwarded-Prefix")); fwd == basePath {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
