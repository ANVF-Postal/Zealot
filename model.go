package main

import (
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// -----------------------------------------------------------------------------
// UI model
// -----------------------------------------------------------------------------

type model struct {
	cfg          Config
	room         string
	roomExplicit   bool
	historyEnabled bool
	user           User
	onlineCount    int
	onlineUsers  []User
	usersOpen    bool
	usersScroll  int
	usersFrame   int

	splashOpen                       bool
	splashScroll                     int
	splashScrollDragging             bool
	splashScrollDragOffset           int
	splashScrollbarHoldDirection     int
	splashScrollbarHoldGeneration    int
	usersScrollDragging bool
	usersScrollDragOffset int
	usersScrollbarHoldDirection int
	usersScrollbarHoldGeneration int
	userActivity map[string]int

	client *wireClient

	messages []Packet
	renderedMessageIDs []string
	hoveredMessageID string

	// Rendered rows are cached for the current terminal width. New messages
	// append only their own rows; a width change rebuilds the cache once.
	renderedLines    []string
	renderCacheWidth int
	renderCacheValid bool

	typing      map[string]User
	typingOrder []string

	input          textarea.Model
	inputSelecting bool
	rainbowMode    bool

	width  int
	height int
	scroll int

	scrollDragging bool
	scrollDragOffset int
	scrollbarHoldDirection int
	scrollbarHoldGeneration int

	connected          bool
	status             string
	notificationsReady bool

	quitConfirm           bool
	quitConfirmGeneration int
	sendFeedback           bool
	sendFeedbackGeneration int

	typingGeneration int
	isTyping         bool

	styles uiStyles
}

func newModel(
	cfg Config,
	room string,
	roomExplicit bool,
	client *wireClient,
) model {
	user := User{
		Name:  cfg.Name,
		Color: cfg.UserColor,
	}

	input := textarea.New()
	// Ctrl+A selects the entire composer; Home remains available for line start.
	input.KeyMap.LineStart.SetKeys("home")
	input.KeyMap.SelectAll.SetKeys("ctrl+a")

	input.Prompt = "" // Render the prompt ourselves so wrapped rows stay plain.
	input.Placeholder = "Type a message…"
	input.CharLimit = maxMessage
	input.ShowLineNumbers = false

	if cfg.InputHeight < 1 {
		cfg.InputHeight = 1
	}
	if cfg.InputHeight > 8 {
		cfg.InputHeight = 8
	}
	input.SetHeight(cfg.InputHeight)

	bg := lipgloss.Color(cfg.Background)
	textStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(cfg.TextColor)).
		Background(bg)
	inputStyles := input.Styles()
	inputStyles.Focused.Base = textStyle
	inputStyles.Blurred.Base = textStyle
	inputStyles.Focused.CursorLine = textStyle
	inputStyles.Blurred.CursorLine = textStyle
	inputStyles.Focused.EndOfBuffer = textStyle
	inputStyles.Blurred.EndOfBuffer = textStyle
	inputStyles.Focused.Text = textStyle
	inputStyles.Blurred.Text = textStyle

	// Match the placeholder to the keyboard-shortcut hint at the bottom.
	placeholderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707786")).
		Background(bg).
		Faint(false).
		Bold(false).
		Italic(false)
	inputStyles.Focused.Placeholder = placeholderStyle
	inputStyles.Blurred.Placeholder = placeholderStyle
	inputStyles.Cursor.Color = lipgloss.Color(cfg.AccentColor)
	inputStyles.Cursor.Blink = false
	input.SetStyles(inputStyles)

	input.Focus()

	// The cursor starts static. We only enable blinking once the
	// user has actually entered text.

	return model{
		cfg:          cfg,
		room:         room,
		roomExplicit: roomExplicit,
		user:         user,
		client:       client,
		typing:       make(map[string]User),
		userActivity: make(map[string]int),
		input:        input,
		connected:    true,
		status:       "Connected",
		styles:       makeStyles(cfg),
	}
}

func (m model) Init() tea.Cmd {
	// Do NOT start textarea.Blink here.
	//
	// That would make an otherwise idle Zealot constantly repaint
	// the terminal/tmux pane.
	return tea.Batch(
		readPacket(m.client),
		tea.Tick(10*time.Second, func(time.Time) tea.Msg {
			return notificationsReadyMsg{}
		}),
	)
}

func readPacket(
	client *wireClient,
) tea.Cmd {
	return func() tea.Msg {
		if client.hasInitial {
			client.hasInitial = false
			return messageReceivedMsg(client.initialPacket)
		}
		var packet Packet

		if err := client.decoder.Decode(&packet); err != nil {
			return connectionClosedMsg{}
		}

		return messageReceivedMsg(packet)
	}
}

// -----------------------------------------------------------------------------
// Cursor behavior
// -----------------------------------------------------------------------------

func (m *model) startCursorBlink() tea.Cmd {
	if m.input.Value() == "" {
		return nil
	}

	styles := m.input.Styles()
	if !styles.Cursor.Blink {
		styles.Cursor.Blink = true
		m.input.SetStyles(styles)
		return textarea.Blink
	}

	return nil
}

func (m *model) stopCursorBlink() {
	if m.input.Value() == "" {
		styles := m.input.Styles()
		styles.Cursor.Blink = false
		m.input.SetStyles(styles)
	}
}
