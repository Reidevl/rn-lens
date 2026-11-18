//go:build server

package main

import (
	"net"
	"strings"
	"testing"
)

func TestPortCheckRefusesABusyOrInvalidPort(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "[::]:0") // like a Lens bound to every IPv6 address
	if err != nil {
		ln, err = (&net.ListenConfig{}).Listen(t.Context(), "tcp", "0.0.0.0:0")
		if err != nil {
			t.Fatal(err)
		}
	}
	defer func() { _ = ln.Close() }()
	busy := ln.Addr().(*net.TCPAddr).Port
	if err := checkPort("localhost", busy); err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("checkPort(busy) = %v", err)
	}
	if err := checkPort("localhost", 99999); err == nil {
		t.Fatal("checkPort(99999) = nil")
	}

	free, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := free.Addr().(*net.TCPAddr).Port
	_ = free.Close()
	if err := checkPort("localhost", port); err != nil {
		t.Fatalf("checkPort(free) = %v", err)
	}
}
