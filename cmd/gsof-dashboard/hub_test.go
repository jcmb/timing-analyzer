package main

import (
	"testing"

	"timing-analyzer/internal/core"
)

func TestValidateStreamRequest_TCPListenEmptyHost(t *testing.T) {
	cfg := core.Config{IP: "tcp", Host: "", Port: 5018}
	if err := validateStreamRequest(cfg, false); err != nil {
		t.Fatalf("TCP listen (empty host) must validate without -allow-private-gsof-targets: %v", err)
	}
}

func TestValidateStreamRequest_TCPDialLoopbackBlockedWithoutAllowPrivate(t *testing.T) {
	cfg := core.Config{IP: "tcp", Host: "127.0.0.1", Port: 5018}
	if err := validateStreamRequest(cfg, false); err == nil {
		t.Fatal("expected error for loopback TCP dial in hub mode without allowPrivate")
	}
}

func TestValidateStreamRequest_TCPDialLoopbackAllowedWithAllowPrivate(t *testing.T) {
	cfg := core.Config{IP: "tcp", Host: "127.0.0.1", Port: 5018}
	if err := validateStreamRequest(cfg, true); err != nil {
		t.Fatalf("loopback TCP dial should validate when allowPrivate: %v", err)
	}
}
