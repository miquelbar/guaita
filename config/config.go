package config

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
)

// AppConfig holds persistent local configuration for Guaita.
type AppConfig struct {
	Port             string `json:"port,omitempty"`
	SessionDays      int    `json:"session_days,omitempty"`
	TLS              *bool  `json:"tls,omitempty"`
	AdminAuth        *bool  `json:"admin_auth,omitempty"`
	TLSCert          string `json:"tls_cert,omitempty"`
	TLSKey           string `json:"tls_key,omitempty"`
	NtfyTopic        string `json:"ntfy_topic,omitempty"`
	NtfyServer       string `json:"ntfy_server,omitempty"`
	TelegramBotToken string `json:"telegram_bot_token,omitempty"`
	TelegramChatID   string `json:"telegram_chat_id,omitempty"`
}

// DefaultPath returns ~/.config/guaita/config.json.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "guaita", "config.json"), nil
}

// Load reads AppConfig from path. If the file does not exist, it returns an empty AppConfig and nil error.
func Load(path string) (AppConfig, error) {
	var cfg AppConfig
	if path == "" {
		return cfg, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}

	// Enforce owner-only permissions on existing config file containing secrets
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		log.Printf("Warning: %s had overly permissive mode %o; tightening to 0600", path, perm)
		_ = os.Chmod(path, 0o600)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if len(data) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Save writes AppConfig atomically to path with 0700 directory and 0600 file permissions.
func Save(path string, cfg AppConfig) error {
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(dir, 0o700)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmpFile, err := os.CreateTemp(dir, "config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if err := tmpFile.Chmod(0o600); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// UpdateTelegram loads the existing config at path (if any), updates Telegram fields, and saves it securely.
func UpdateTelegram(path, botToken, chatID string) error {
	cfg, err := Load(path)
	if err != nil {
		cfg = AppConfig{}
	}
	cfg.TelegramBotToken = botToken
	cfg.TelegramChatID = chatID
	return Save(path, cfg)
}
