package spotifylauncher

import (
    "errors"
    "os/exec"
)

func commandExists(cmd string) bool {
    _, err := exec.LookPath(cmd)
    return err == nil
}

func DetectSpotify() (string, error) {
    if commandExists("flatpak") {
        if exec.Command("flatpak", "info", "com.spotify.Client").Run() == nil {
            return "flatpak", nil
        }
    }

    if commandExists("spotify") {
        return "binary", nil
    }

    return "", errors.New("spotify not found")
}

func LaunchSpotify() error {
    kind, err := DetectSpotify()
    if err != nil {
        return err
    }

    switch kind {
    case "flatpak":
        return exec.Command("flatpak", "run", "com.spotify.Client").Start()
    case "binary":
        return exec.Command("spotify").Start()
    }

    return errors.New("unknown spotify installation")
}