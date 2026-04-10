package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/ayanacorp/proxymon/internal/balancer"
)

type Settings struct {
	ProxyAddr             string        `json:"proxy_addr"`
	SelectedInterfaceKeys []string      `json:"selected_interface_keys"`
	Mode                  balancer.Mode `json:"mode"`
	WinProxyAuto          bool          `json:"win_proxy_auto"`
}

type Store struct {
	path       string
	legacyPath string
}

func NewStore() (*Store, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}

	return &Store{
		path:       filepath.Join(configDir, "proxymon", "settings.json"),
		legacyPath: filepath.Join(configDir, "venn-combine-connection", "settings.json"),
	}, nil
}

func (s *Store) Load() (Settings, error) {
	if err := s.migrateLegacy(); err != nil {
		return Settings{}, err
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Settings{}, nil
		}
		return Settings{}, err
	}

	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return Settings{}, err
	}

	return settings, nil
}

func (s *Store) Save(settings Settings) error {
	if err := s.migrateLegacy(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0o644)
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) migrateLegacy() error {
	if s.legacyPath == "" || s.legacyPath == s.path {
		return nil
	}
	if _, err := os.Stat(s.path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	data, err := os.ReadFile(s.legacyPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0o644)
}
