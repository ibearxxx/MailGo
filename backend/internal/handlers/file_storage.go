package handlers

import (
	"os"
	"path/filepath"
)

func mailgoDataDir() (string, error) {
	if dir := os.Getenv("MAILGO_DATA_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mailgo"), nil
}
