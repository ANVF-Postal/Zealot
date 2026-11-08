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
	mentionUsernames []string
	usersOpen    bool
	usersScroll  int
	usersFrame   int
	usersAnimationRunning bool

	splashOpen                       bool
	themesOpen                       bool
	themeEntries                     []themeListEntry
	themeSelected                    int
	themeScroll                      int
	themeDetailsScroll               int
	themeError                       string
	themeEditError                   string
	themeDirty                       bool
	themeSavedPreset                 ThemePreset
	themeEditField                   string
	themeComposerBackup              string
	themePacking                     bool
	themePackNaming                  bool
	themePackSelected                map[string]bool
	themeLeftWidth                   int
	themeDividerDragging             bool
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
	messageByID map[string]Packet
	replyIDsByMessageID map[string][]string
	messageRowsByID map[string]messageRowRange
	// messageRowRanges stores the absolute row range of each message in history
	// order. The full history needs these lightweight counts for scrolling,
	// but styled rows are kept only around the current viewport.
	messageRowRanges []messageRowRange
	renderWindowStart int
	renderWindowEnd int
	renderWindowValid bool
	// Interaction geometry is calculated on demand for messages the pointer
	// reaches, instead of laying out hitboxes for the entire history on resize.
	messageBodyRangesByID map[string][]messageBodyRange
	messageLinkRangesByID map[string][][]messageLinkRange
	hoveredMessageID string
	mouseMotion *mouseMotionState
	mouseMotionFrameGeneration int
	replyTargetID string
	highlightedMessageID string
	navigationHighlightGeneration int
	lastMouseX int
	lastMouseY int
	mousePositionKnown bool

	// renderedLines is indexed by absolute history row, but styled strings
	// are populated only for the visible window and its small prefetch buffer.
	renderedLines []string
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
	quitting            bool
	status             string
	notificationsReady bool

	quitConfirm           bool
	quitConfirmGeneration int
	sendFeedback           bool
	sendFeedbackGeneration int

	typingGeneration   int
	typingTimerPending bool
	lastTypingAt       time.Time
	isTyping           bool

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

	bg := lipgloss.Color(cfg.Theme.Background)
	textStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(cfg.Theme.TextColor)).
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

	// Text selection is part of the active theme, not Bubbles' default palette.
	selectionStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(cfg.Theme.SelectionBackgroundColor))
	inputStyles.Focused.Selection = selectionStyle
	inputStyles.Blurred.Selection = selectionStyle

	// Match the placeholder to the keyboard-shortcut hint at the bottom.
	placeholderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(cfg.Theme.MutedTextColor)).
		Background(bg).
		Faint(false).
		Bold(false).
		Italic(false)
	inputStyles.Focused.Placeholder = placeholderStyle
	inputStyles.Blurred.Placeholder = placeholderStyle
	inputStyles.Cursor.Color = lipgloss.Color(cfg.Theme.AccentColor)
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
		mentionUsernames: []string{user.Name},
		client:       client,
		mouseMotion:  &mouseMotionState{},
		typing:       make(map[string]User),
		userActivity: make(map[string]int),
		input:        input,
		connected:    true,
		status:       "Connected",
		styles:       makeStyles(cfg.Theme),
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

const historyBatchSize = 128

func readPacket(
	client *wireClient,
) tea.Cmd {
	return func() tea.Msg {
		if client.hasInitial {
			client.hasInitial = false
			return messageReceivedMsg(client.initialPacket)
		}

		if client.historyReplay {
			packets := make([]Packet, 0, historyBatchSize)
			for len(packets) < historyBatchSize {
				var packet Packet
				if err := client.decoder.Decode(&packet); err != nil {
					if len(packets) == 0 {
						return connectionClosedMsg{}
					}
					return historyBatchReceivedMsg{Packets: packets}
				}
				if packet.Type == "history_end" {
					client.historyReplay = false
					return historyBatchReceivedMsg{Packets: packets, Complete: true}
				}
				packets = append(packets, packet)
			}
			return historyBatchReceivedMsg{Packets: packets}
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
