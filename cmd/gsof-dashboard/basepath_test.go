package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStripHTTPBasePath_fullURI(t *testing.T) {
	var got string
	h := stripHTTPBasePath("/GSOF", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Path
	}))
	req := httptest.NewRequest(http.MethodGet, "/GSOF/api/config", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if got != "/api/config" {
		t.Fatalf("path = %q, want /api/config", got)
	}
}

func TestStripHTTPBasePath_forwardedPrefixAlreadyStripped(t *testing.T) {
	var got string
	h := stripHTTPBasePath("/GSOF", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Path
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Forwarded-Prefix", "/GSOF")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if got != "/api/config" {
		t.Fatalf("path = %q, want /api/config", got)
	}
}

func TestResolveHTTPBasePath(t *testing.T) {
	if got := resolveHTTPBasePath("", "/GSOF/"); got != "/GSOF" {
		t.Fatalf("got %q", got)
	}
	if got := resolveHTTPBasePath("/apps", "/GSOF"); got != "/apps" {
		t.Fatalf("got %q", got)
	}
}
