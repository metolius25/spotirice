package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Colors defines the color scheme for the UI.
type Colors struct {
	Header        string `toml:"header"`
	TrackPlaying  string `toml:"track_playing"`
	TrackPaused   string `toml:"track_paused"`
	Artist        string `toml:"artist"`
	ProgressBar   string `toml:"progress_bar"`
	Status        string `toml:"status"`
	Error         string `toml:"error"`
}

// DefaultColors provides a fallback color scheme.
func DefaultColors() *Colors {
	return &Colors{
		Header:        "#00FFFF", // cyan
		TrackPlaying:  "#00FF00", // green
		TrackPaused:   "#FFFF00", // yellow
		Artist:        "#FFFFFF", // white
		ProgressBar:   "#FFFFFF", // white
		Status:        "#808080", // grey
		Error:         "#FF0000", // red
	}
}

// LoadColors reads the config.toml file and returns a Colors struct.
func LoadColors() (*Colors, error) {
	path := filepath.Join(os.Getenv("HOME"), ".config", "spotirice", "config.toml")
	
	// Start with default colors
	colors := DefaultColors()

	// If the config file exists, decode it and override defaults
	if _, err := os.Stat(path); err == nil {
		if _, err := toml.DecodeFile(path, colors); err != nil {
			return nil, err
		}
	}

	return colors, nil
}
