package spotifylauncher

import (
	"errors"
	"os/exec"
	"runtime"
)

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// DetectSpotify checks for Spotify installation on the current platform
func DetectSpotify() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		// macOS: Check for Spotify.app
		if commandExists("open") {
			return "macos", nil
		}
	case "windows":
		// Windows: Check for Spotify in common locations
		if commandExists("spotify.exe") {
			return "windows", nil
		}
		// Try AppData location
		return "windows-store", nil
	default:
		// Linux and others
		if commandExists("flatpak") {
			if exec.Command("flatpak", "info", "com.spotify.Client").Run() == nil {
				return "flatpak", nil
			}
		}
		if commandExists("spotify") {
			return "binary", nil
		}
		if commandExists("snap") {
			if exec.Command("snap", "list", "spotify").Run() == nil {
				return "snap", nil
			}
		}
	}

	return "", errors.New("spotify not found")
}

// LaunchSpotify attempts to launch Spotify on the current platform
func LaunchSpotify() error {
	kind, err := DetectSpotify()
	if err != nil {
		return err
	}

	switch kind {
	case "macos":
		return exec.Command("open", "-a", "Spotify").Start()
	case "windows", "windows-store":
		return exec.Command("cmd", "/c", "start", "spotify:").Start()
	case "flatpak":
		return exec.Command("flatpak", "run", "com.spotify.Client").Start()
	case "snap":
		return exec.Command("snap", "run", "spotify").Start()
	case "binary":
		return exec.Command("spotify").Start()
	}

	return errors.New("unknown spotify installation")
}
