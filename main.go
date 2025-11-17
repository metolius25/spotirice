package main

import (
	"context"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zmb3/spotify/v2"

	"github.com/metolius25/spotirice/internal/auth"
	"github.com/metolius25/spotirice/internal/config"
	"github.com/metolius25/spotirice/internal/ui/root"
)

type clientMsg struct{ Client *spotify.Client }
type errMsg struct{ Err error }

type model struct {
	client *spotify.Client
	status string
	colors *config.Colors
}

func initialModel(colors *config.Colors) model {
	return model{status: "Authenticating...", colors: colors}
}

// Trigger authentication only.
func (m model) Init() tea.Cmd {
	return startAuthCmd
}

func startAuthCmd() tea.Msg {
	client, err := auth.Authenticate()
	if err != nil {
		return errMsg{err}
	}
	return clientMsg{client}
}

func (m model) runDeviceAutoSelect() tea.Cmd {
	return func() tea.Msg {
		devices, err := m.client.PlayerDevices(context.Background())
		if err != nil {
			return errMsg{Err: err}
		}

		if len(devices) == 0 {
			// Tell RootModel to display this later
			return clientMsg{Client: m.client}
		}

		var valid *spotify.PlayerDevice
		for _, d := range devices {
			if !d.Restricted && (d.Type == "Computer" || d.Type == "Smartphone") {
				valid = &d
				break
			}
		}

		if valid != nil {
			_ = m.client.TransferPlayback(context.Background(), valid.ID, false)
		}

		return clientMsg{Client: m.client}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}

	case clientMsg:
		// First time: store client & init device selection
		if m.client == nil {
			m.client = msg.Client
			m.status = "Authenticated! Detecting devices..."
			return m, m.runDeviceAutoSelect()
		}

		// Second time: all done → switch to root UI
		return root.NewRootModel(msg.Client, m.colors)

	case errMsg:
		m.status = "Error: " + msg.Err.Error()
	}

	return m, nil
}

func (m model) View() string {
	return "Spotirice\n" + m.status
}

func main() {
	colors, err := config.LoadColors()
	if err != nil {
		log.Fatal("Failed to load colors:", err)
	}

	p := tea.NewProgram(
		initialModel(colors),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if err := p.Start(); err != nil {
		log.Fatal(err)
	}
}
