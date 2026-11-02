package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// -----------------------------------------------------------------------------
// UI update
// -----------------------------------------------------------------------------

func (m model) Update(
	msg tea.Msg,
) (tea.Model, tea.Cmd) {

	// -------------------------------------------------------------------------
	// Window resize
	// -------------------------------------------------------------------------

	if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
		oldWidth := m.width
		m.width = sizeMsg.Width
		m.height = sizeMsg.Height
		if m.inputSelecting {
			m.input.EndSelection()
			m.inputSelecting = false
		}
		m.resizeInput()
		if oldWidth != m.width || !m.renderCacheValid {
			m.rebuildMessageCache()
		}
		m.clampScroll()
		// The roster's row count can change when the terminal width changes,
		// and its viewport changes when the height changes. Keep the saved
		// offset within the new valid range so clipped names reappear naturally
		// when the window is expanded again.
		m.clampUsersScroll()
		m.clampSplashScroll()
		m.splashScrollDragging = false
		m.splashScrollbarHoldDirection = 0
		m.splashScrollbarHoldGeneration++
		m.usersScrollDragging = false
		m.usersScrollbarHoldDirection = 0
		m.usersScrollbarHoldGeneration++

		return m, nil
	}

	// -------------------------------------------------------------------------
	// Network messages
	// -------------------------------------------------------------------------

	if received, ok := msg.(messageReceivedMsg); ok {
		packet := Packet(received)

		switch packet.Type {

		case "online":
			count, err := strconv.Atoi(packet.Text)
			if err == nil {
				m.onlineCount = count
			}

		case "users":
			// Roster broadcasts keep this snapshot current for both the online
			// panel and the /who command. They are not chat messages themselves.
			m.onlineUsers = append([]User(nil), packet.Users...)
			sort.Slice(m.onlineUsers, func(i, j int) bool {
				return strings.ToLower(m.onlineUsers[i].Name) < strings.ToLower(m.onlineUsers[j].Name)
			})
			m.clampUsersScroll()

		case "message", "system", "me", "letter":
			m.appendMessage(packet)
			if packet.Type == "message" && packet.User.Name != m.user.Name {
				m.userActivity[packet.User.Name] = 6
			}

		case "identity":
			if packet.User.Name != "" {
				m.user.Name = packet.User.Name
			}
			if packet.User.Color != "" {
				m.user.Color = packet.User.Color
			}

		case "welcome":
			// The server may have appended a numeric suffix to avoid a duplicate
			// username. Use that canonical identity for outgoing messages and
			// notification filtering.
			if packet.User.Name != "" {
				m.user = packet.User
			}
			m.historyEnabled = packet.AddHistory

		case "history_state":
			m.historyEnabled = packet.AddHistory

		case "typing":
			key := packet.User.Name

			if packet.Typing {
				if _, exists := m.typing[key]; !exists {
					m.typingOrder = append(m.typingOrder, key)
				}
				m.typing[key] = packet.User
			} else {
				delete(m.typing, key)

				for i, name := range m.typingOrder {
					if name == key {
						m.typingOrder = append(m.typingOrder[:i], m.typingOrder[i+1:]...)
						break
					}
				}
			}

		case "history_end":
			// Give the terminal a short settling period after the initial
			// history replay before enabling notifications.
			readyCmd := tea.Tick(750*time.Millisecond, func(time.Time) tea.Msg {
				return notificationsReadyMsg{}
			})
			return m, tea.Batch(readPacket(m.client), readyCmd)
		}

		next := readPacket(m.client)
		var commands []tea.Cmd
		commands = append(commands, next)
		if packet.Type == "message" && packet.User.Name != m.user.Name {
			commands = append(commands, tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return usersAnimationMsg{} }))
			if m.notificationsReady && m.cfg.Bell {
				commands = append(commands, terminalBellCmd())
			}
		}
		return m, tea.Batch(commands...)
	}

	if _, ok := msg.(usersAnimationMsg); ok {
		m.usersFrame++
		active := m.usersOpen
		for name, remaining := range m.userActivity {
			if remaining <= 1 {
				delete(m.userActivity, name)
			} else {
				m.userActivity[name] = remaining - 1
				active = true
			}
		}
		if active {
			return m, tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
				return usersAnimationMsg{}
			})
		}
		return m, nil
	}

	if _, ok := msg.(connectionClosedMsg); ok {
		m.connected = false
		m.status = "Disconnected"

		return m, nil
	}

	if _, ok := msg.(notificationsReadyMsg); ok {
		m.notificationsReady = true
		return m, nil
	}

	if timeoutMsg, ok := msg.(quitConfirmTimeoutMsg); ok {
		if timeoutMsg.generation == m.quitConfirmGeneration {
			m.quitConfirm = false
		}
		return m, nil
	}

	if timeoutMsg, ok := msg.(sendFeedbackTimeoutMsg); ok {
		if timeoutMsg.generation == m.sendFeedbackGeneration {
			m.sendFeedback = false
		}
		return m, nil
	}

	if timeoutMsg, ok := msg.(messageNavigationHighlightTimeoutMsg); ok {
		if timeoutMsg.generation == m.navigationHighlightGeneration {
			m.highlightedMessageID = ""
			m.rebuildMessageCache()
		}
		return m, nil
	}

	if clearMsg, ok := msg.(typingClearMsg); ok {
		if clearMsg.generation == m.typingGeneration {
			m.isTyping = false
			m.sendTyping(false)
		}

		return m, nil
	}

	// -------------------------------------------------------------------------
	// Scrollbar mouse interaction
	// -------------------------------------------------------------------------

	switch mouse := msg.(type) {
	case tea.MouseClickMsg:
		// Begin a textarea selection before handling other clickable UI. The
		// coordinate helper accounts for the composer frame and its first-row
		// prompt, which is rendered outside the Bubbles textarea.
		if mouse.Button == tea.MouseLeft && !m.usersOpen && !m.splashOpen &&
			m.inputSelectionContains(mouse.X, mouse.Y) {
			localX, localY := m.inputSelectionCoordinates(mouse.X, mouse.Y)
			m.input.BeginSelection(localX, localY)
			m.inputSelecting = true
			return m, nil
		}

		// Footer actions occupy the final terminal row. Their hitboxes are
		// calculated by the same layout helper used to render the row.
		if m.statusBarVisible() && mouse.Button == tea.MouseLeft && mouse.Y == m.height-1 {
			_, targets := m.renderStatusBar(m.usersOpen, m.splashOpen)
			if targets.hasSend && mouse.X >= targets.sendStart && mouse.X < targets.sendEnd {
				m.sendCurrentMessage()
				m.sendFeedback = true
				m.sendFeedbackGeneration++
				generation := m.sendFeedbackGeneration
				return m, tea.Tick(350*time.Millisecond, func(time.Time) tea.Msg {
					return sendFeedbackTimeoutMsg{generation: generation}
				})
			}
			if targets.hasPageScroll && mouse.X >= targets.pgUpStart && mouse.X < targets.pgUpEnd {
				m.scrollPage(true)
				return m, nil
			}
			if targets.hasPageScroll && mouse.X >= targets.pgDnStart && mouse.X < targets.pgDnEnd {
				m.scrollPage(false)
				return m, nil
			}
			if mouse.X >= targets.quitStart && mouse.X < targets.quitEnd {
				if m.quitConfirm {
					m.quitConfirm = false
					m.quitConfirmGeneration++
					m.sendTyping(false)
					_ = m.client.conn.Close()
					return m, tea.Quit
				}
				m.quitConfirm = true
				m.quitConfirmGeneration++
				generation := m.quitConfirmGeneration
				return m, tea.Tick(4*time.Second, func(time.Time) tea.Msg {
					return quitConfirmTimeoutMsg{generation: generation}
				})
			}
		}
		if (m.usersOpen || m.splashOpen) && mouse.Button == tea.MouseLeft && mouse.Y == 0 && mouse.X >= m.width-lipgloss.Width("[X]") && mouse.X < m.width {
			m.usersOpen = false
			m.splashOpen = false
			m.usersScrollDragging = false
			m.usersScrollbarHoldDirection = 0
			m.usersScrollbarHoldGeneration++
			m.splashScrollDragging = false
			m.splashScrollbarHoldDirection = 0
			m.splashScrollbarHoldGeneration++
			return m, nil
		}
		if mouse.Button == tea.MouseLeft && mouse.Y == 0 && mouse.X >= 0 && mouse.X < lipgloss.Width("ZEALOT") {
			if m.splashOpen {
				m.splashOpen = false
				m.splashScrollDragging = false
				m.splashScrollbarHoldDirection = 0
				m.splashScrollbarHoldGeneration++
			} else {
				m.splashOpen = true
				m.usersOpen = false
				m.splashScroll = 0
				m.splashScrollDragging = false
				m.splashScrollbarHoldDirection = 0
				m.splashScrollbarHoldGeneration++
				m.usersScrollDragging = false
				m.usersScrollbarHoldDirection = 0
				m.usersScrollbarHoldGeneration++
				m.scrollDragging = false
				m.scrollbarHoldDirection = 0
				m.scrollbarHoldGeneration++
			}
			return m, nil
		}
		if !m.usersOpen && !m.splashOpen && mouse.Button == tea.MouseLeft && mouse.Y == 0 && mouse.X >= m.width-lipgloss.Width(fmt.Sprintf("Online: %d", m.onlineCount)) && mouse.X < m.width {
			m.usersOpen = true
			m.usersScroll = 0
			m.usersScrollDragging = false
			m.usersScrollbarHoldDirection = 0
			m.usersScrollbarHoldGeneration++
			m.scrollDragging = false
			m.scrollbarHoldDirection = 0
			m.scrollbarHoldGeneration++
			_ = m.client.send(Packet{Type: "who", Room: m.room})
			return m, tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return usersAnimationMsg{} })
		}
		if m.usersOpen && mouse.Button == tea.MouseLeft && mouse.X == m.chatScrollbarX() && m.maxUsersScroll() > 0 {
			visible := m.usersPanelHeight()
			firstTrackY := 3
			topArrowY := 2 // aligned with the Online users heading
			bottomArrowY := firstTrackY + visible - 1
			switch {
			case mouse.Y == topArrowY:
				m.usersScrollbarHoldDirection = -1
				m.usersScrollbarHoldGeneration++
				m.usersScroll -= 3
				m.clampUsersScroll()
				return m, m.usersScrollbarHoldCmd(m.usersScrollbarHoldGeneration)
			case mouse.Y == bottomArrowY:
				m.usersScrollbarHoldDirection = 1
				m.usersScrollbarHoldGeneration++
				m.usersScroll += 3
				m.clampUsersScroll()
				return m, m.usersScrollbarHoldCmd(m.usersScrollbarHoldGeneration)
			case mouse.Y > topArrowY && mouse.Y < bottomArrowY:
				top, thumbHeight, _ := m.usersScrollbarThumb()
				trackPosition := mouse.Y - firstTrackY
				if trackPosition >= top && trackPosition < top+thumbHeight {
					m.usersScrollDragging = true
					m.usersScrollDragOffset = trackPosition - top
					return m, nil
				}
				m.setUsersScrollFromTrack(trackPosition)
				return m, nil
			}
		}
		if m.splashOpen && mouse.Button == tea.MouseLeft && mouse.X == m.chatScrollbarX() && m.maxSplashScroll() > 0 {
			visible := m.splashPanelHeight()
			firstTrackY := 3
			topArrowY := 2
			bottomArrowY := 1 + visible
			switch {
			case mouse.Y == topArrowY:
				m.splashScrollbarHoldDirection = -1
				m.splashScrollbarHoldGeneration++
				m.splashScroll -= 3
				m.clampSplashScroll()
				return m, m.splashScrollbarHoldCmd(m.splashScrollbarHoldGeneration)
			case mouse.Y == bottomArrowY:
				m.splashScrollbarHoldDirection = 1
				m.splashScrollbarHoldGeneration++
				m.splashScroll += 3
				m.clampSplashScroll()
				return m, m.splashScrollbarHoldCmd(m.splashScrollbarHoldGeneration)
			case mouse.Y > topArrowY && mouse.Y < bottomArrowY:
				top, thumbHeight, _ := m.splashScrollbarThumb()
				trackPosition := mouse.Y - firstTrackY
				if trackPosition >= top && trackPosition < top+thumbHeight {
					m.splashScrollDragging = true
					m.splashScrollDragOffset = trackPosition - top
					return m, nil
				}
				m.setSplashScrollFromTrack(trackPosition)
				return m, nil
			}
		}
		if !m.usersOpen && !m.splashOpen && mouse.Button == tea.MouseLeft && m.scrollbarContains(mouse.X, mouse.Y) && m.maxScroll() > 0 {
			top, thumbHeight, trackHeight := m.scrollbarThumb()
			trackY := mouse.Y - 3 // first draggable track row; arrows occupy y=2 and the final row
			if trackY < 0 { trackY = 0 }
			if trackY >= trackHeight { trackY = trackHeight - 1 }

			switch {
			case mouse.Y == 2:
				m.scrollbarHoldDirection = 1
				m.scrollbarHoldGeneration++
				m.scroll += 3
				m.clampScroll()
				return m, m.scrollbarHoldCmd(m.scrollbarHoldGeneration)
			case mouse.Y == 1+m.messageHeight():
				m.scrollbarHoldDirection = -1
				m.scrollbarHoldGeneration++
				m.scroll -= 3
				m.clampScroll()
				return m, m.scrollbarHoldCmd(m.scrollbarHoldGeneration)
			case trackY >= top && trackY < top+thumbHeight:
				m.scrollDragging = true
				m.scrollDragOffset = trackY - top
				return m, nil
			default:
				m.setScrollFromTrack(trackY)
				return m, nil
			}
		}
		if mouse.Button == tea.MouseLeft && !m.usersOpen && !m.splashOpen {
			if targetID := m.messageLinkAt(mouse.X, mouse.Y); targetID != "" {
				return m, m.navigateToMessage(targetID)
			}
			if id := m.messageIDAt(mouse.X, mouse.Y); id != "" {
				m.beginReply(id)
				return m, nil
			}
		}

	case tea.MouseMotionMsg:
		m.lastMouseX = mouse.X
		m.lastMouseY = mouse.Y
		m.mousePositionKnown = true
		if m.inputSelecting {
			localX, localY := m.inputSelectionCoordinates(mouse.X, mouse.Y)
			m.input.ExtendSelection(localX, localY)
			m.refreshMessageHover()
			return m, nil
		}
		if m.splashOpen && m.splashScrollDragging {
			_, thumbHeight, trackHeight := m.splashScrollbarThumb()
			target := mouse.Y - 3 - m.splashScrollDragOffset
			if target < 0 { target = 0 }
			if target > trackHeight-thumbHeight { target = trackHeight-thumbHeight }
			m.setSplashScrollFromThumbTop(target)
			return m, nil
		}
		if m.usersOpen && m.usersScrollDragging {
			_, thumbHeight, trackHeight := m.usersScrollbarThumb()
			target := mouse.Y - 3 - m.usersScrollDragOffset
			if target < 0 { target = 0 }
			if target > trackHeight-thumbHeight { target = trackHeight-thumbHeight }
			m.setUsersScrollFromThumbTop(target)
			return m, nil
		}
		if m.scrollDragging && !m.usersOpen && !m.splashOpen {
			_, thumbHeight, trackHeight := m.scrollbarThumb()
			target := mouse.Y - 3 - m.scrollDragOffset
			if target < 0 { target = 0 }
			if target > trackHeight-thumbHeight { target = trackHeight-thumbHeight }
			m.setScrollFromThumbTop(target)
			return m, nil
		}
		m.refreshMessageHover()

	case tea.MouseReleaseMsg:
		if m.inputSelecting {
			m.input.EndSelection()
			m.inputSelecting = false
		}
		m.splashScrollDragging = false
		m.splashScrollbarHoldDirection = 0
		m.splashScrollbarHoldGeneration++
		m.scrollDragging = false
		m.scrollbarHoldDirection = 0
		m.scrollbarHoldGeneration++
		m.usersScrollDragging = false
		m.usersScrollbarHoldDirection = 0
		m.usersScrollbarHoldGeneration++
		return m, nil

	case usersScrollbarHoldMsg:
		if m.usersOpen && mouse.generation == m.usersScrollbarHoldGeneration && m.usersScrollbarHoldDirection != 0 {
			m.usersScroll += m.usersScrollbarHoldDirection * 3
			m.clampUsersScroll()
			if m.usersScroll == 0 || m.usersScroll == m.maxUsersScroll() {
				m.usersScrollbarHoldDirection = 0
				return m, nil
			}
			return m, m.usersScrollbarHoldCmd(mouse.generation)
		}
		return m, nil
	case splashScrollbarHoldMsg:
		if m.splashOpen && mouse.generation == m.splashScrollbarHoldGeneration && m.splashScrollbarHoldDirection != 0 {
			m.splashScroll += m.splashScrollbarHoldDirection * 3
			m.clampSplashScroll()
			if m.splashScroll == 0 || m.splashScroll == m.maxSplashScroll() {
				m.splashScrollbarHoldDirection = 0
				return m, nil
			}
			return m, m.splashScrollbarHoldCmd(mouse.generation)
		}
		return m, nil
	case scrollbarHoldMsg:
		if !m.usersOpen && !m.splashOpen && mouse.generation == m.scrollbarHoldGeneration && m.scrollbarHoldDirection != 0 {
			m.scroll += m.scrollbarHoldDirection * 3
			m.clampScroll()
			if m.scroll == 0 || m.scroll == m.maxScroll() {
				m.scrollbarHoldDirection = 0
				return m, nil
			}
			return m, m.scrollbarHoldCmd(mouse.generation)
		}
		return m, nil
	}

	// -------------------------------------------------------------------------
	// Mouse wheel
	// -------------------------------------------------------------------------

	if mouseMsg, ok := msg.(tea.MouseWheelMsg); ok {
		if m.splashOpen {
			switch mouseMsg.Button {
			case tea.MouseWheelUp:
				m.splashScroll -= 3
			case tea.MouseWheelDown:
				m.splashScroll += 3
			}
			m.clampSplashScroll()
			return m, nil
		}
		if m.usersOpen {
			switch mouseMsg.Button {
			case tea.MouseWheelUp:
				m.usersScroll -= 3
			case tea.MouseWheelDown:
				m.usersScroll += 3
			}
			m.clampUsersScroll()
			return m, nil
		}
		switch mouseMsg.Button {
		case tea.MouseWheelUp:
			m.scroll += 3
			m.clampScroll()
			return m, nil
		case tea.MouseWheelDown:
			m.scroll -= 3
			m.clampScroll()
			return m, nil
		}
	}

	// -------------------------------------------------------------------------
	// Keyboard shortcuts
	// -------------------------------------------------------------------------

	handledNewline := false

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {

		case "esc", "escape":
			if m.splashOpen {
				m.splashOpen = false
				m.splashScrollDragging = false
				m.splashScrollbarHoldDirection = 0
				m.splashScrollbarHoldGeneration++
				return m, nil
			}
			if m.usersOpen {
				m.usersOpen = false
				m.usersScrollDragging = false
				m.usersScrollbarHoldDirection = 0
				m.usersScrollbarHoldGeneration++
				return m, nil
			}

		case "ctrl+o":
			if m.usersOpen {
				m.usersOpen = false
				m.usersScrollDragging = false
				m.usersScrollbarHoldDirection = 0
				m.usersScrollbarHoldGeneration++
				return m, nil
			}

		case "ctrl+c", "ctrl+q":
			m.sendTyping(false)
			_ = m.client.conn.Close()

			return m, tea.Quit

		case "pgup":
			m.scrollPage(true)
			return m, nil

		case "pgdown":
			m.scrollPage(false)
			return m, nil

		case "ctrl+home":
			if m.splashOpen {
				m.splashScroll = 0
				return m, nil
			}
			m.scroll = m.maxScroll()
			return m, nil

		case "ctrl+end":
			if m.splashOpen {
				m.splashScroll = m.maxSplashScroll()
				return m, nil
			}
			m.scroll = 0
			return m, nil

		case "backspace":
			if m.replyTargetID != "" && m.input.Line() == 0 && m.input.Column() == 0 {
				m.cancelReply()
				return m, nil
			}

		case "shift+enter", "ctrl+j":
			m.input.InsertString("\n")
			handledNewline = true

		case "enter":
			m.sendCurrentMessage()
			return m, nil
		}
	}

	// -------------------------------------------------------------------------
	// Pass the keyboard event to the textarea.
	//
	// THIS IS IMPORTANT.
	//
	// The textarea must receive the key event or the character won't
	// actually appear in the input box.
	// -------------------------------------------------------------------------

	var inputCmd tea.Cmd

	if !handledNewline {
		m.input, inputCmd = m.input.Update(msg)
	}
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && m.replyTargetID != "" && m.input.Value() == "" {
		if keyMsg.String() == "backspace" || keyMsg.String() == "delete" {
			m.cancelReply()
		}
	}
	if m.cfg.AdaptiveHeight {
		m.clampScroll()
		m.clampUsersScroll()
		m.clampSplashScroll()
	}

	// -------------------------------------------------------------------------
	// Typing detection
	// -------------------------------------------------------------------------

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		if m.connected && isTextInputKey(keyMsg) {

			m.typingGeneration++

			generation := m.typingGeneration

			// Start blinking only when there is actual text.
			blinkCmd := m.startCursorBlink()

			if !m.isTyping {
				m.isTyping = true
				m.sendTyping(true)
			}

			// Consider the user to have stopped typing after
			// 1.2 seconds without another editing key.
			typingCmd := tea.Tick(
				1200*time.Millisecond,
				func(time.Time) tea.Msg {
					return typingClearMsg{
						generation: generation,
					}
				},
			)

			return m, tea.Batch(
				inputCmd,
				blinkCmd,
				typingCmd,
			)
		}
	}

	// If the textarea is empty, make sure the cursor isn't blinking.
	if m.input.Value() == "" {
		m.stopCursorBlink()
	}

	return m, inputCmd
}

// scrollPage applies the same viewport movement for keyboard and footer clicks.
func (m *model) scrollPage(up bool) {
	if m.splashOpen {
		step := max(1, m.splashPanelHeight()/2)
		if up {
			m.splashScroll -= step
		} else {
			m.splashScroll += step
		}
		m.clampSplashScroll()
		return
	}
	if m.usersOpen {
		step := max(1, m.usersPanelHeight())
		if up {
			m.usersScroll -= step
		} else {
			m.usersScroll += step
		}
		m.clampUsersScroll()
		return
	}
	if up {
		m.scroll += m.messageHeight() / 2
	} else {
		m.scroll -= m.messageHeight() / 2
	}
	m.clampScroll()
}

func isTextInputKey(
	msg tea.KeyPressMsg,
) bool {
	// Printable characters.
	if len(msg.Text) > 0 {
		for _, r := range msg.Text {
			if r >= 32 {
				return true
			}
		}
	}

	// Editing keys.
	switch msg.String() {
	case "backspace", "delete", "shift+enter", "ctrl+j":
		return true
	}

	return false
}

func (m model) sendTyping(
	typing bool,
) {
	_ = m.client.send(Packet{
		Type:   "typing",
		Room:   m.room,
		User:   m.user,
		Typing: typing,
	})
}
