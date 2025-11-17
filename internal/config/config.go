package config

import (
    "encoding/json"
    "os"
    "path/filepath"
)

type Credentials struct {
    ClientID     string `json:"client_id"`
    ClientSecret string `json:"client_secret"`
}

func LoadCredentials() (*Credentials, error) {
    path := filepath.Join(os.Getenv("HOME"), ".config", "spotirice", "credentials.json")

    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    var creds Credentials
    err = json.Unmarshal(data, &creds)
    if err != nil {
        return nil, err
    }

    return &creds, nil
}
