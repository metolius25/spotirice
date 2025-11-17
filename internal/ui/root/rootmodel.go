package root

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zmb3/spotify/v2"

	"github.com/metolius25/spotirice/internal/config"
)

type statusMsg string
type errMsg struct{ Err error }
type tickMsg struct{}

type playerStateMsg struct {
	TrackName  string
	ArtistName string
	ProgressMs int
	DurationMs int
	Playing    bool
}

type RootModel struct {
	client *spotify.Client
	status string
	colors *config.Colors

	// player state
	trackName  string
	artistName string
	progressMs int
	durationMs int
	isPlaying  bool

	width int
}

// Init is run when RootModel becomes active. It starts polling state and the ticker.
func (m RootModel) Init() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return tea.Batch(
		pollStateCmd(m.client),
		tickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

func pollStateCmd(c *spotify.Client) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		state, err := c.PlayerState(ctx)
		if err != nil || state == nil || state.Item == nil {
			// silently ignore; keep last known state
			return nil
		}

		track := state.Item
		artist := ""
		if len(track.Artists) > 0 {
			artist = track.Artists[0].Name
		}

		return playerStateMsg{
			TrackName:  track.Name,
			ArtistName: artist,
			ProgressMs: int(state.Progress),
			DurationMs: int(track.Duration),
			Playing:    state.Playing,
		}
	}
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width

	case tea.KeyMsg:
		switch msg.String() {
		case "p", " ":
			if m.client == nil {
				return m, nil
			}
			if m.isPlaying {
				return m, pauseCmd(m.client)
			}
			return m, resumePlaybackCmd(m.client)

		case "n":
			if m.client == nil {
				return m, nil
			}
			return m, nextCmd(m.client)

		case "b":
			if m.client == nil {
				return m, nil
			}
			return m, prevCmd(m.client)

		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case tea.MouseMsg:
		// Ignore mouse-down events to avoid double triggering
		if msg.Action != tea.MouseActionRelease {
			return m, nil
		}

		controlRow := 5
		if msg.Y == controlRow && m.client != nil {
			x := msg.X
			switch {
			case x >= 0 && x <= 4:
				if m.isPlaying {
					return m, pauseCmd(m.client)
				}
				return m, resumePlaybackCmd(m.client)

			case x >= 8 && x <= 10:
				return m, prevCmd(m.client)

			case x >= 14 && x <= 16:
				return m, nextCmd(m.client)
			}
		}

	case tickMsg:
		if m.client == nil {
			return m, tickCmd()
		}
		return m, tea.Batch(
			pollStateCmd(m.client),
			tickCmd(),
		)

	case playerStateMsg:
		m.trackName = msg.TrackName
		m.artistName = msg.ArtistName
		m.progressMs = msg.ProgressMs
		m.durationMs = msg.DurationMs
		m.isPlaying = msg.Playing

	case statusMsg:
		m.status = string(msg)

	case errMsg:
		m.status = "Error: " + msg.Err.Error()
	}

	return m, nil
}

func (m RootModel) View() string {
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.colors.Header))
	trackPlayingStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.colors.TrackPlaying))
	trackPausedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.colors.TrackPaused))
	artistStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.colors.Artist))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.colors.Error))
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.colors.Status))

	header := headerStyle.Render("== NOW PLAYING =======================================")

	// Track line
	trackLine := "> (no track)"
	artistLine := "  (no artist)"
	if m.trackName != "" {
		if m.isPlaying {
			trackLine = "> " + trackPlayingStyle.Render(m.trackName)
		} else {
			trackLine = "> " + trackPausedStyle.Render(m.trackName)
		}
	}
	if m.artistName != "" {
		artistLine = "  " + artistStyle.Render(m.artistName)
	}

	// Status / info (bottom of header area)
	statusLine := statusStyle.Render(m.status)
	if strings.HasPrefix(m.status, "Error:") {
		statusLine = errorStyle.Render(m.status)
	}

	// Controls
	playIcon := "▶"
	if m.isPlaying {
		playIcon = "⏸"
	}
	controlsLine := fmt.Sprintf("[ %s ]   [⏮]   [⏭]", playIcon)

	// Progress bar (auto width)
	barLine := m.renderProgressLine()

	lines := []string{
		header,
		"",
		trackLine,
		artistLine,
		"",
		controlsLine,
		barLine,
		"",
		statusLine,
	}

	return strings.Join(lines, "\n")
}

func (m RootModel) renderProgressLine() string {
	if m.durationMs <= 0 {
		return ""
	}

	progressStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.colors.ProgressBar))

	// Calculate bar width based on terminal width.
	// Leave ~20 chars for timer and spacing.
	w := m.width
	if w <= 0 {
		w = 80
	}
	barWidth := w - 20
	if barWidth < 10 {
		barWidth = 10
	}

	ratio := float64(m.progressMs) / float64(m.durationMs)
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}

	filled := int(ratio * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	empty := barWidth - filled

	left := strings.Repeat("─", filled)
	right := strings.Repeat("·", empty)

	cur := formatTime(m.progressMs)
	total := formatTime(m.durationMs)

	return progressStyle.Render(fmt.Sprintf("%s%s  %s/%s", left, right, cur, total))
}

func formatTime(ms int) string {
	if ms < 0 {
		ms = 0
	}
	totalSec := ms / 1000
	min := totalSec / 60
	sec := totalSec % 60
	return fmt.Sprintf("%d:%02d", min, sec)
}

// NewRootModel builds the root UI and starts polling.
func NewRootModel(c *spotify.Client, colors *config.Colors) (RootModel, tea.Cmd) {
	m := RootModel{
		client: c,
		status: "Authenticated. Use p/space to play/pause, n/b to skip.",
		colors: colors,
	}
	return m, m.Init()
}

// ------------------ Commands ------------------

func ensureActiveDevice(c *spotify.Client) error {
	ctx := context.Background()

	devices, err := c.PlayerDevices(ctx)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		return fmt.Errorf("no devices found; open Spotify on a device")
	}

	var device *spotify.PlayerDevice
	for i := range devices {
		d := &devices[i]
		if d.Restricted {
			continue
		}
		if d.Type != "Computer" && d.Type != "Smartphone" && d.Type != "Speaker" {
			continue
		}
		if device == nil || d.Active {
			device = d
			if d.Active {
				break
			}
		}
	}

	if device == nil {
		return fmt.Errorf("no controllable device found (avoid Web Player)")
	}

	if !device.Active {
		if err := c.TransferPlayback(ctx, device.ID, false); err != nil {
			return err
		}
		// DO NOT auto-play — this causes "restriction violated"
		return nil
	}

	return nil
}

func resumePlaybackCmd(c *spotify.Client) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		if err := ensureActiveDevice(c); err != nil {
			return errMsg{Err: err}
		}

		// Only call Play() if the player is currently paused.
		state, err := c.PlayerState(ctx)
		if err != nil {
			return errMsg{Err: err}
		}

		if state != nil && !state.Playing {
			if err := c.Play(ctx); err != nil {
				return errMsg{Err: err}
			}
		}

		return statusMsg("Resumed playback.")
	}
}

func pauseCmd(c *spotify.Client) tea.Cmd {
	return func() tea.Msg {
		if err := c.Pause(context.Background()); err != nil {
			return errMsg{Err: err}
		}
		return statusMsg("Paused.")
	}
}

func nextCmd(c *spotify.Client) tea.Cmd {
	return func() tea.Msg {
		if err := c.Next(context.Background()); err != nil {
			return errMsg{Err: err}
		}
        return statusMsg("Skipped to next track.")
	}
}

func prevCmd(c *spotify.Client) tea.Cmd {
	return func() tea.Msg {
		if err := c.Previous(context.Background()); err != nil {
			return errMsg{Err: err}
		}
		return statusMsg("Went back to previous track.")
	}
}
