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
type clearStatusMsg struct{}
type searchResultsMsg struct {
	Tracks []spotify.FullTrack
}

type playerStateMsg struct {
	TrackName  string
	ArtistName string
	ProgressMs int
	DurationMs int
	Playing    bool
	ID         spotify.ID
	Liked      bool
	Volume     int
}

type RootModel struct {
	client *spotify.Client
	status string
	colors *config.Colors

	// player state
	trackName       string
	artistName      string
	progressMs      int
	durationMs      int
	isPlaying       bool
	hasInitialState bool
	currentTrackID  spotify.ID
	trackIsLiked    bool

	// playback state
	volume int // 0-100

	// UI state
	showHelp            bool
	burstTicksRemaining int // countdown for burst tick mode (10 ticks = 1 second at 100ms)
	version             string

	// Search state
	isSearching   bool
	searchInput   textinput.Model
	searchResults []spotify.FullTrack
	searchCursor  int

	width  int
	height int
}

// clearStatusCmd returns a command that clears the status after 5 seconds
func clearStatusCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return clearStatusMsg{} })
}

// Init is run when RootModel becomes active. It starts polling state and the ticker.
func (m RootModel) Init() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return tea.Batch(
		tea.WindowSize(),
		pollStateCmd(m.client),
		tickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

func fastTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func pollStateCmd(c *spotify.Client) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		state, err := c.PlayerState(ctx)
		if err != nil || state == nil || state.Item == nil {
			return statusMsg("Waiting for playback...")
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
			ID:         track.ID,
			Liked:      len(liked) > 0 && liked[0],
			Volume:     int(state.Device.Volume),
		}
	}
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		// Handle search mode input
		if m.isSearching {
			switch msg.String() {
			case "esc":
				m.isSearching = false
				m.searchResults = nil
				m.searchCursor = 0
				return m, nil
			case "enter":
				if len(m.searchResults) > 0 && m.searchCursor < len(m.searchResults) {
					// Play the selected track
					track := m.searchResults[m.searchCursor]
					m.isSearching = false
					m.searchResults = nil
					m.searchCursor = 0
					return m, playTrackCmd(m.client, track.URI)
				} else if m.searchInput.Value() != "" {
					// Perform search
					return m, searchCmd(m.client, m.searchInput.Value())
				}
			case "up":
				if m.searchCursor > 0 {
					m.searchCursor--
				}
				return m, nil
			case "down":
				if m.searchCursor < len(m.searchResults)-1 {
					m.searchCursor++
				}
				return m, nil
			default:
				// Pass input to textinput
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				return m, cmd
			}
			return m, nil
		}

		// If help is showing, any key closes it
		if m.showHelp {
			if msg.String() == "esc" || msg.String() == "?" {
				m.showHelp = false
				return m, nil
			}
		}

		switch msg.String() {
		case "/", "s":
			// Enter search mode
			m.isSearching = true
			m.searchInput = textinput.New()
			m.searchInput.Placeholder = "Search for songs..."
			m.searchInput.SetValue("")
			m.searchInput.Focus()
			m.searchResults = nil
			m.searchCursor = 0
			return m, m.searchInput.Cursor.BlinkCmd()

		case "?":
			m.showHelp = !m.showHelp
			return m, nil

		case "p", " ":
			if m.client == nil {
				return m, nil
			}
			m.burstTicksRemaining = 10 // Fast polling for 1 second
			if m.isPlaying {
				return m, pauseCmd(m.client)
			}
			return m, resumePlaybackCmd(m.client)

		case "n":
			if m.client == nil {
				return m, nil
			}
			m.burstTicksRemaining = 10
			return m, nextCmd(m.client)

		case "b":
			if m.client == nil {
				return m, nil
			}
			m.burstTicksRemaining = 10
			return m, prevCmd(m.client)

		case "l":
			if m.currentTrackID != "" {
				m.burstTicksRemaining = 10
				return m, toggleLikeCmd(m.client, m.currentTrackID, m.trackIsLiked)
			}

		case "+", "=":
			if m.client != nil {
				newVol := m.volume + 10
				if newVol > 100 {
					newVol = 100
				}
				m.burstTicksRemaining = 10
				return m, setVolumeCmd(m.client, newVol)
			}

		case "-", "_":
			if m.client != nil {
				newVol := m.volume - 10
				if newVol < 0 {
					newVol = 0
				}
				m.burstTicksRemaining = 10
				return m, setVolumeCmd(m.client, newVol)
			}

		case "left":
			if m.client != nil && m.progressMs > 0 {
				newPos := m.progressMs - 10000
				if newPos < 0 {
					newPos = 0
				}
				m.burstTicksRemaining = 10
				return m, seekCmd(m.client, newPos)
			}

		case "right":
			if m.client != nil && m.durationMs > 0 {
				newPos := m.progressMs + 10000
				if newPos > m.durationMs {
					newPos = m.durationMs - 1000
				}
				m.burstTicksRemaining = 10
				return m, seekCmd(m.client, newPos)
			}

		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case tea.MouseMsg:
		// Handle mouse wheel scrolling in search mode
		if m.isSearching && len(m.searchResults) > 0 {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				if m.searchCursor > 0 {
					m.searchCursor--
				}
				return m, nil
			case tea.MouseButtonWheelDown:
				if m.searchCursor < len(m.searchResults)-1 {
					m.searchCursor++
				}
				return m, nil
			}
		}

		// Ignore mouse-down events to avoid double triggering
		if msg.Action != tea.MouseActionRelease {
			return m, nil
		}

		// --- Calculate control button positions ---
		// This is a bit of a hack, but it's the most reliable way with the current
		// view structure. We reconstruct the layout logic to find the button positions.

		// Calculate the Y position of the control row
		// header(1) + container border(1) + trackInfo(2) + separator(1) + progress bar(1)
		controlRow := 1 + 1 + 2 + 1 + 1

		if msg.Y == controlRow && m.client != nil {
			// Build the controls string as in View()
			playIcon := "▶"
			if m.isPlaying {
				playIcon = "⏸"
			}

			heart := "♡"
			if m.trackIsLiked {
				heart = "♥"
			}

			controlsText := fmt.Sprintf(" [ 🔍 Search ]  [ %s ]  [ ⏮ ]  [ ⏭ ]  [ %s ] ", playIcon, heart)
			controlsWidth := lipgloss.Width(controlsText)

			// Container width is terminal width minus borders
			containerWidth := m.width - lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				GetHorizontalBorderSize()

			// Controls are centered in the container
			padding := (containerWidth - controlsWidth) / 2

			// Get mouse X relative to the start of the controls string
			relativeX := msg.X - padding

			// Button positions within " [ 🔍 Search ]  [ P ]  [ ⏮ ]  [ ⏭ ]  [ ♡ ] "
			// Position:                   1-12         15-19  22-26  29-33  36-40
			switch {
			case relativeX >= 1 && relativeX <= 12: // Search
				m.isSearching = true
				m.searchInput = textinput.New()
				m.searchInput.Placeholder = "Search for songs..."
				m.searchInput.SetValue("")
				m.searchInput.Focus()
				m.searchResults = nil
				m.searchCursor = 0
				return m, m.searchInput.Cursor.BlinkCmd()

			case relativeX >= 15 && relativeX <= 19: // Play/Pause
				m.burstTicksRemaining = 10
				if m.isPlaying {
					return m, pauseCmd(m.client)
				}
				return m, resumePlaybackCmd(m.client)

			case relativeX >= 22 && relativeX <= 26: // Previous
				m.burstTicksRemaining = 10
				return m, prevCmd(m.client)

			case relativeX >= 29 && relativeX <= 33: // Next
				m.burstTicksRemaining = 10
				return m, nextCmd(m.client)

			case relativeX >= 36 && relativeX <= 40: // Heart/Like
				if m.currentTrackID != "" {
					m.burstTicksRemaining = 10
					return m, toggleLikeCmd(m.client, m.currentTrackID, m.trackIsLiked)
				}
			}

		}

		// --- Handle progress bar clicks ---
		// Progress bar is on row: header(1) + container border(1) + trackInfo(2) + separator(1) = row 5
		progressRow := 1 + 1 + 2 + 1

		if msg.Y == progressRow && m.client != nil && m.durationMs > 0 {
			// Calculate progress bar dimensions (matching renderProgressLine)
			w := m.width
			if w <= 0 {
				w = 80
			}
			barWidth := w - 4 - 15 // container border + padding + timer width
			if barWidth < 10 {
				barWidth = 10
			}

			// Progress bar format: "cur/total [bar]"
			cur := formatTime(m.progressMs)
			total := formatTime(m.durationMs)
			timerWidth := len(cur) + 1 + len(total) + 1 // "cur/total " with space

			// Calculate where the bar starts (centered in container)
			containerWidth := m.width - lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				GetHorizontalBorderSize()

			progressLineWidth := timerWidth + barWidth
			padding := (containerWidth - progressLineWidth) / 2

			// Click position relative to bar start
			barStartX := padding + timerWidth - 2
			barClickPos := msg.X - barStartX

			if barClickPos >= 0 && barClickPos < barWidth {
				// Calculate the seek position
				ratio := float64(barClickPos) / float64(barWidth)
				if ratio < 0 {
					ratio = 0
				}
				if ratio > 1 {
					ratio = 1
				}
				seekPos := int(ratio * float64(m.durationMs))
				m.burstTicksRemaining = 10
				return m, seekCmd(m.client, seekPos)
			}
		}
	case tickMsg:
		// Determine next tick rate based on burst mode
		var nextTick tea.Cmd
		if m.burstTicksRemaining > 0 {
			m.burstTicksRemaining--
			nextTick = fastTickCmd()
		} else {
			nextTick = tickCmd()
			// Only smooth progress during normal ticks (not during burst mode)
			if m.client != nil && m.isPlaying && m.hasInitialState {
				m.progressMs += 1000
				if m.progressMs > m.durationMs {
					m.progressMs = m.durationMs
				}
			}
		}

		return m, tea.Batch(
			pollStateCmd(m.client),
			nextTick,
		)

	case playerStateMsg:
		m.hasInitialState = true
		m.trackName = msg.TrackName
		m.artistName = msg.ArtistName
		m.progressMs = msg.ProgressMs
		m.durationMs = msg.DurationMs
		m.isPlaying = msg.Playing

		m.currentTrackID = msg.ID
		m.trackIsLiked = msg.Liked
		m.volume = msg.Volume

	case statusMsg:
		m.status = string(msg)
		return m, clearStatusCmd()

	case clearStatusMsg:
		m.status = ""

	case errMsg:
		m.status = "Error: " + msg.Err.Error()
		return m, clearStatusCmd()

	case searchResultsMsg:
		m.searchResults = msg.Tracks
		m.searchCursor = 0
	}

	return m, nil
}

func (m RootModel) View() string {
	// Show help screen if enabled
	if m.showHelp {
		return m.renderHelpScreen()
	}

	// Show search screen if searching
	if m.isSearching {
		return m.renderSearchScreen()
	}

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
	header := headerStyle.Render(fmt.Sprintf(" Spotirice v%s", m.version))

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

	controls := fmt.Sprintf(" [ 🔍 Search ]  [ %s ]  [ ⏮ ]  [ ⏭ ]  [ %s ] ", playIcon, heart)

	// Volume bar
	volumeLine := fmt.Sprintf("🔊 %d%%", m.volume)

	// Progress Bar
	barLine := m.renderProgressLine()

	// Status
	statusLine := statusStyle.Render(m.status + "  |  ? for help")
	if strings.HasPrefix(m.status, "Error:") {
		statusLine = errorStyle.Render(m.status)
	}

	// Assembly
	ui := lipgloss.JoinVertical(lipgloss.Center,
		trackInfo,
		"",
		barLine,
		controls,
		volumeLine,
		"",
		statusLine,
	)

	// Make it fit the container - fill terminal width and height
	w := m.width - containerStyle.GetHorizontalBorderSize()
	// Height: terminal height minus header (1 line) minus container border (2 lines)
	h := m.height - 1 - containerStyle.GetVerticalBorderSize()
	if h < 1 {
		h = lipgloss.Height(ui) // Fallback to content height
	}
	renderedUI := containerStyle.Width(w).Height(h).Render(ui)

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		renderedUI,
	)
}

func (m RootModel) renderHelpScreen() string {
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.colors.Header)).
		Bold(true).
		Padding(0, 1)

	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.colors.Header)).
		Padding(1, 2)

	helpText := `
Keyboard Controls
─────────────────
  p / Space    Play/Pause
  n            Next track
  b            Previous track
  l            Like/Unlike song

  + / =        Volume up (+10%)
  - / _        Volume down (-10%)

  ← / →        Seek -/+10 seconds

  s / /        Search for songs
  ?            Toggle help
  q / Ctrl+C   Quit

Press ESC or ? to close this screen
`

	header := headerStyle.Render(" Spotirice Help")
	helpBox := containerStyle.Render(helpText)

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		helpBox,
	)
}

func (m RootModel) renderSearchScreen() string {
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.colors.Header)).
		Bold(true).
		Padding(0, 1)

	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.colors.Header)).
		Padding(1, 2)

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.colors.TrackPlaying)).
		Bold(true)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.colors.Artist))

	header := headerStyle.Render(" 🔍 Search")
	inputLine := "Search: " + m.searchInput.View()

	var resultLines []string
	resultLines = append(resultLines, inputLine, "")

	if len(m.searchResults) == 0 {
		if m.searchInput.Value() != "" {
			resultLines = append(resultLines, "Press Enter to search...")
		} else {
			resultLines = append(resultLines, "Type to search for songs, then press Enter")
		}
	} else {
		// Scrollable results - calculate max visible based on terminal height
		// Reserve lines for: header(1) + border(2) + padding(2) + search input(1) + blank(1) + results header(1) + blank(1) + footer(2)
		reservedLines := 11
		maxVisible := m.height - reservedLines
		if maxVisible < 3 {
			maxVisible = 3 // Minimum 3 results
		}
		if maxVisible > len(m.searchResults) {
			maxVisible = len(m.searchResults)
		}

		start := 0
		if m.searchCursor >= maxVisible {
			start = m.searchCursor - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(m.searchResults) {
			end = len(m.searchResults)
		}

		resultLines = append(resultLines, fmt.Sprintf("Results %d-%d of %d (↑/↓ to scroll, Enter to play):", start+1, end, len(m.searchResults)), "")

		if start > 0 {
			resultLines = append(resultLines, normalStyle.Render("  ↑ more results above"))
		}

		for i := start; i < end; i++ {
			track := m.searchResults[i]
			artist := ""
			if len(track.Artists) > 0 {
				artist = track.Artists[0].Name
			}
			line := fmt.Sprintf("  %s - %s", track.Name, artist)
			if i == m.searchCursor {
				line = selectedStyle.Render("▶ " + line[2:])
			} else {
				line = normalStyle.Render(line)
			}
			resultLines = append(resultLines, line)
		}

		if end < len(m.searchResults) {
			resultLines = append(resultLines, normalStyle.Render("  ↓ more results below"))
		}
	}

	resultLines = append(resultLines, "", "Press ESC to cancel")

	content := strings.Join(resultLines, "\n")

	// Make container fill terminal width and height
	w := m.width - containerStyle.GetHorizontalBorderSize()
	h := m.height - 1 - containerStyle.GetVerticalBorderSize()
	if h < 1 {
		h = lipgloss.Height(content)
	}
	searchBox := containerStyle.Width(w).Height(h).Render(content)

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		searchBox,
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

	// Use distinct characters: ━ for filled (progress), ─ for empty (remaining)
	left := progressStyle.Render(strings.Repeat("━", filled))
	right := emptyStyle.Render(strings.Repeat("─", empty))

	cur := formatTime(m.progressMs)
	total := formatTime(m.durationMs)

	return fmt.Sprintf("%s/%s %s%s", cur, total, left, right)
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
func NewRootModel(c *spotify.Client, colors *config.Colors, version string) (RootModel, tea.Cmd) {
	m := RootModel{
		client:  c,
		status:  "Authenticated. Use p/space to play/pause, n/b to skip.",
		colors:  colors,
		version: version,
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

func setVolumeCmd(c *spotify.Client, volume int) tea.Cmd {
	return func() tea.Msg {
		if err := c.Volume(context.Background(), volume); err != nil {
			return errMsg{Err: err}
		}
		return statusMsg(fmt.Sprintf("Volume: %d%%", volume))
	}
}

func seekCmd(c *spotify.Client, positionMs int) tea.Cmd {
	return func() tea.Msg {
		if err := c.Seek(context.Background(), positionMs); err != nil {
			return errMsg{Err: err}
		}
		return pollStateCmd(c)()
	}
}

func searchCmd(c *spotify.Client, query string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		results, err := c.Search(ctx, query, spotify.SearchTypeTrack)
		if err != nil {
			return errMsg{Err: err}
		}
		if results.Tracks == nil || len(results.Tracks.Tracks) == 0 {
			return statusMsg("No results found")
		}
		// Return up to 10 results
		tracks := results.Tracks.Tracks
		if len(tracks) > 10 {
			tracks = tracks[:10]
		}
		return searchResultsMsg{Tracks: tracks}
	}
}

func playTrackCmd(c *spotify.Client, uri spotify.URI) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		opts := &spotify.PlayOptions{
			URIs: []spotify.URI{uri},
		}
		if err := c.PlayOpt(ctx, opts); err != nil {
			return errMsg{Err: err}
		}
		return statusMsg("Playing selected track")
	}
}
