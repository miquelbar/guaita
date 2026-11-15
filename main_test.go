package main

import (
	"encoding/hex"
	"net"
	"strings"
	"testing"
)

func TestGenerateToken(t *testing.T) {
	t1 := generateToken()
	t2 := generateToken()

	if len(t1) != 32 {
		t.Errorf("expected token length 32, got %d", len(t1))
	}
	if _, err := hex.DecodeString(t1); err != nil {
		t.Errorf("token is not valid hex: %v", err)
	}
	if t1 == t2 {
		t.Errorf("two generated tokens should not be identical: %s == %s", t1, t2)
	}
}

func TestGetLocalIP(t *testing.T) {
	ip := getLocalIP()
	parsed := net.ParseIP(ip)
	if parsed == nil {
		t.Fatalf("expected valid IP, got %s", ip)
	}
	if parsed.To4() == nil {
		t.Errorf("expected IPv4 address, got %s", ip)
	}
}

func TestRenderQR(t *testing.T) {
	qr, err := renderQR("https://192.168.10.189:8085/?token=6f9800487180f533051a70ef5670f80b")
	if err != nil {
		t.Fatalf("unexpected error rendering QR: %v", err)
	}
	if len(qr) == 0 {
		t.Errorf("expected non-empty QR output")
	}
	lines := strings.Count(qr, "\n")
	if lines > 22 {
		t.Errorf("expected compact terminal QR (<=22 lines), got %d lines", lines)
	}
}
