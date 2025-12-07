package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
)

const tokenFileName = "token.json"

func tokenFilePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not get config dir: %w", err)
	}

	spotiriceDir := filepath.Join(configDir, "spotirice")
	if err := os.MkdirAll(spotiriceDir, 0700); err != nil {
		return "", fmt.Errorf("could not create config dir: %w", err)
	}

	return filepath.Join(spotiriceDir, tokenFileName), nil
}

func SaveToken(tok *oauth2.Token) error {
	path, err := tokenFilePath()
	if err != nil {
		return err
	}

	data, err := json.Marshal(tok)
	if err != nil {
		return fmt.Errorf("could not marshal token: %w", err)
	}

	return os.WriteFile(path, data, 0600)
}

func LoadToken() (*oauth2.Token, error) {
	path, err := tokenFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("could not unmarshal token: %w", err)
	}

	return &tok, nil
}

func TokenExists() bool {
	path, err := tokenFilePath()
	if err != nil {
		return false
	}

	_, err = os.Stat(path)
	return err == nil
}
