package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStripHTTPBasePath_fullURI(t *testing.T) {
	var got string
	h := stripHTTPBasePath("/baseline", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Path
	}))
	req := httptest.NewRequest(http.MethodGet, "/baseline/api/config", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if got != "/api/config" {
		t.Fatalf("path = %q, want /api/config", got)
	}
}

func TestStripHTTPBasePath_forwardedPrefixAlreadyStripped(t *testing.T) {
	var got string
	h := stripHTTPBasePath("/baseline", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Path
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Forwarded-Prefix", "/baseline")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if got != "/api/config" {
		t.Fatalf("path = %q, want /api/config", got)
	}
}

func TestPublicPath(t *testing.T) {
	if got := publicPath("/baseline", "/events"); got != "/baseline/events" {
		t.Fatalf("got %q", got)
	}
	if got := publicPath("", "/events"); got != "/events" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveHTTPBasePath(t *testing.T) {
	if got := resolveHTTPBasePath("", "/baseline/"); got != "/baseline" {
		t.Fatalf("got %q", got)
	}
	if got := resolveHTTPBasePath("/apps", "/baseline"); got != "/apps" {
		t.Fatalf("got %q", got)
	}
}
