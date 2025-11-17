package root

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zmb3/spotify/v2"

	"github.com/metolius25/spotirice/internal/config"
)

type statusMsg string
type errMsg struct{ Err error }
type tickMsg struct{}
type searchResultMsg struct {
	Result *spotify.SearchResult
}

type playerStateMsg struct {
	TrackName  string
	ArtistName string
	ProgressMs int
	DurationMs int
	Playing    bool
	ID         spotify.ID
	Liked      bool
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
	hasInitialState bool
	currentTrackID  spotify.ID
	trackIsLiked    bool



	// search state
	isSearching   bool
	searchInput   textinput.Model
	searchResults *spotify.SearchResult
	searchCursor  int

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
            return nil
        }

        track := state.Item
        artist := ""
        if len(track.Artists) > 0 {
            artist = track.Artists[0].Name
        }

        // check if liked
        liked, _ := c.UserHasTracks(ctx, track.ID)

        return playerStateMsg{
            TrackName:  track.Name,
            ArtistName: artist,
            ProgressMs: int(state.Progress),
            DurationMs: int(track.Duration),
            Playing:    state.Playing,
            // new:
            ID:         track.ID,
            Liked:      len(liked) > 0 && liked[0],
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

		
    	case "l":
            if m.currentTrackID != "" {
                return m, toggleLikeCmd(m.client, m.currentTrackID, m.trackIsLiked)
            }
			
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case tea.MouseMsg:
		// Ignore mouse-down events to avoid double triggering
		if msg.Action != tea.MouseActionRelease {
			return m, nil
		}

		// --- Calculate control button positions ---
		// This is a bit of a hack, but it's the most reliable way with the current
		// view structure. We reconstruct the layout logic to find the button positions.

		// 1. Calculate the Y position of the control row
		// header(1) + container border(1) + trackInfo(2) + separator(1) + progress bar(1)
		controlRow := 1 + 1 + 2 + 1 + 1

		if msg.Y == controlRow && m.client != nil {
			// 2. Calculate the X position of the controls string
			heart := "♡"
        	if m.trackIsLiked {
            	heart = "♥"
        	}

			
			
			playIcon := "▶"
			if m.isPlaying {
				playIcon = "⏸"
			}
			controlsText := fmt.Sprintf(" [ %s ]  [ ⏮ ]  [ ⏭ ]  [ %s ] ", playIcon, heart)
			controlsWidth := lipgloss.Width(controlsText)

			// Container width is terminal width minus borders
			containerWidth := m.width - lipgloss.NewStyle().
            	Border(lipgloss.RoundedBorder()).
            	GetHorizontalBorderSize()

			// Controls are centered in the container
			padding := (containerWidth - controlsWidth) / 2

			// 3. Get mouse X relative to the start of the controls string
			relativeX := msg.X - padding

			// 4. Check which button was clicked based on their hardcoded positions within the string
			// " [ P ]  [ B ]  [ N ] "
			//   1-5    8-12   15-19
			switch {
			case relativeX >= 1 && relativeX <= 5: // Play/Pause
				if m.isPlaying {
					return m, pauseCmd(m.client)
				}
				return m, resumePlaybackCmd(m.client)

			case relativeX >= 8 && relativeX <= 12: // Previous
				return m, prevCmd(m.client)

			case relativeX >= 15 && relativeX <= 19: // Next
				return m, nextCmd(m.client)


			case relativeX >= 22 && relativeX <= 26:   // heart
            	if m.currentTrackID != "" {
                	return m, toggleLikeCmd(m.client, m.currentTrackID, m.trackIsLiked)
	
			}
		}

	}
	case tickMsg:
		if m.client != nil && m.isPlaying && m.hasInitialState {
			// Only smooth **after first real state**
			m.progressMs += 1000
			if m.progressMs > m.durationMs {
				m.progressMs = m.durationMs
			}
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
		m.isPlaying  = msg.Playing

		m.currentTrackID = msg.ID
		m.trackIsLiked   = msg.Liked

	case statusMsg:
		m.status = string(msg)

	case errMsg:
		m.status = "Error: " + msg.Err.Error()
	}

	return m, nil
}

func (m RootModel) View() string {
	// Styles
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.colors.Header)).
		Bold(true).
		Padding(0, 1)

	trackPlayingStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.colors.TrackPlaying)).
		Bold(true)

	trackPausedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.colors.TrackPaused))

	artistStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.colors.Artist))

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.colors.Error)).
		Bold(true)

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.colors.Status))

	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.colors.Header))

	// Header
	header := headerStyle.Render(" Spotirice: CLI Spotify Controller")

	// Track Info
	trackLine := "No track playing"
	artistLine := ""
	if m.trackName != "" {
		if m.isPlaying {
			trackLine = trackPlayingStyle.Render(m.trackName)
		} else {
			trackLine = trackPausedStyle.Render(m.trackName + " (paused)")
		}
		artistLine = artistStyle.Render(m.artistName)
	}

	trackInfo := lipgloss.JoinVertical(lipgloss.Left,
		trackLine,
		artistLine,
	)

	// Controls
	playIcon := "▶"
	if m.isPlaying {
		playIcon = "⏸"
	}

	// Like Song
	heart := "♡"
	if m.trackIsLiked {
    	heart = "♥"
	}

	controls := fmt.Sprintf(" [ %s ]  [ ⏮ ]  [ ⏭ ]  [ %s ] ", playIcon, heart)

	// Progress Bar
	barLine := m.renderProgressLine()

	// Status
	statusLine := statusStyle.Render(m.status)
	if strings.HasPrefix(m.status, "Error:") {
		statusLine = errorStyle.Render(m.status)
	}

	// Assembly
	ui := lipgloss.JoinVertical(lipgloss.Center,
		trackInfo,
		"",
		barLine,
		controls,
		"",
		statusLine,
	)

	// Make it fit the container
	w := m.width - containerStyle.GetHorizontalBorderSize()
	h := lipgloss.Height(ui)
	renderedUI := containerStyle.Width(w).Height(h).Render(ui)

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		renderedUI,
	)
}

func (m RootModel) renderProgressLine() string {
	if m.durationMs <= 0 {
		return ""
	}

	progressStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.colors.ProgressBar))
	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.colors.Artist)) // Use a dimmer color

	w := m.width
	if w <= 0 {
		w = 80
	}
	// container border + padding + timer width
	barWidth := w - 4 - 15
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

	left := progressStyle.Render(strings.Repeat("█", filled))
	right := emptyStyle.Render(strings.Repeat("█", empty))

	cur := formatTime(m.progressMs)
	total := formatTime(m.durationMs)

	return fmt.Sprintf("%s %s/%s %s", cur, total, left, right)
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

    var active *spotify.PlayerDevice
    var firstValid *spotify.PlayerDevice

    for i := range devices {
        d := &devices[i]

        if d.Restricted {
            continue
        }
        if d.Type != "Computer" && d.Type != "Smartphone" && d.Type != "Speaker" {
            continue
        }

        if firstValid == nil {
            firstValid = d
        }
        if d.Active {
            active = d
            break
        }
    }

    // If we already have an active device → DO NOT TRANSFER.
    if active != nil {
        return nil
    }

    // Only transfer when absolutely required.
    if firstValid != nil {
        return c.TransferPlayback(ctx, firstValid.ID, false)
    }

    return fmt.Errorf("no controllable devices available")
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

func toggleLikeCmd(c *spotify.Client, trackID spotify.ID, currentlyLiked bool) tea.Cmd {
    return func() tea.Msg {
        ctx := context.Background()

        if currentlyLiked {
            // Remove from liked
            if err := c.RemoveTracksFromLibrary(ctx, trackID); err != nil {
                return errMsg{Err: err}
            }
            return statusMsg("Removed from Liked Songs.")
        }

        // Add to liked
        if err := c.AddTracksToLibrary(ctx, trackID); err != nil {
            return errMsg{Err: err}
        }
        return statusMsg("Added to Liked Songs.")
    }
}
