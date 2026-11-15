package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("expected nil error for missing config, got %v", err)
	}
	if cfg.Port != "" || cfg.TelegramBotToken != "" {
		t.Errorf("expected empty config, got %+v", cfg)
	}
}

func TestSaveAndLoadPermissionsAndValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json")
	tlsTrue := true

	want := AppConfig{
		Port:             "9090",
		SessionDays:      14,
		TLS:              &tlsTrue,
		NtfyTopic:        "my-topic",
		TelegramBotToken: "12345:secret-token",
		TelegramChatID:   "987654",
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("expected 0600 permissions, got %o", perm)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got.Port != want.Port || got.SessionDays != want.SessionDays || got.TelegramBotToken != want.TelegramBotToken || got.TelegramChatID != want.TelegramChatID {
		t.Errorf("loaded config mismatch: got %+v, want %+v", got, want)
	}
	if got.TLS == nil || !*got.TLS {
		t.Errorf("expected TLS=true, got %v", got.TLS)
	}
}

func TestUpdateTelegramPreservesOtherFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := Save(path, AppConfig{Port: "8099", NtfyTopic: "keep-me"}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if err := UpdateTelegram(path, "bot:tok", "111222"); err != nil {
		t.Fatalf("UpdateTelegram failed: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got.Port != "8099" || got.NtfyTopic != "keep-me" {
		t.Errorf("expected other fields preserved, got %+v", got)
	}
	if got.TelegramBotToken != "bot:tok" || got.TelegramChatID != "111222" {
		t.Errorf("expected updated telegram fields, got %+v", got)
	}
}
