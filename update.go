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
		m.themeDividerDragging = false
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

	if batch, ok := msg.(historyBatchReceivedMsg); ok {
		m.appendMessages(batch.Packets)
		if batch.Complete {
			readyCmd := tea.Tick(750*time.Millisecond, func(time.Time) tea.Msg {
				return notificationsReadyMsg{}
			})
			return m, tea.Batch(readPacket(m.client), readyCmd)
		}
		return m, readPacket(m.client)
	}

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
			m.refreshMentionUsernames()

		case "message", "system", "me", "letter":
			m.appendMessage(packet)
			if packet.Type == "message" && packet.User.Name != m.user.Name {
				m.userActivity[packet.User.Name] = 6
			}

		case "identity":
			oldName := m.user.Name
			if packet.User.Name != "" {
				m.user.Name = packet.User.Name
			}
			if packet.User.Color != "" {
				m.user.Color = packet.User.Color
			}
			if oldName != m.user.Name {
				m.refreshMentionUsernames()
			}
			if oldName != m.user.Name && m.renderCacheValid {
				m.rebuildMessageCache()
			}

		case "welcome":
			// The server may have appended a numeric suffix to avoid a duplicate
			// username. Use that canonical identity for outgoing messages and
			// notification filtering.
			oldName := m.user.Name
			if packet.User.Name != "" {
				m.user = packet.User
			}
			if oldName != m.user.Name {
				m.refreshMentionUsernames()
			}
			if oldName != m.user.Name && m.renderCacheValid {
				m.rebuildMessageCache()
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
		if packet.Type == "users" {
			if animationCmd := m.startUsersAnimation(); animationCmd != nil {
				commands = append(commands, animationCmd)
			}
		}
		if packet.Type == "message" && packet.User.Name != m.user.Name {
			if animationCmd := m.startUsersAnimation(); animationCmd != nil {
				commands = append(commands, animationCmd)
			}
			if m.notificationsReady && m.cfg.Bell {
				commands = append(commands, terminalBellCmd())
			}
		}
		return m, tea.Batch(commands...)
	}

	if _, ok := msg.(usersAnimationMsg); ok {
		m.usersAnimationRunning = false
		m.usersFrame++
		for name, remaining := range m.userActivity {
			if remaining <= 1 {
				delete(m.userActivity, name)
			} else {
				m.userActivity[name] = remaining - 1
			}
		}
		if animationCmd := m.startUsersAnimation(); animationCmd != nil {
			return m, animationCmd
		}
		return m, nil
	}

	if _, ok := msg.(connectionClosedMsg); ok {
		if m.quitting {
			return m, tea.Quit
		}
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
			previousID := m.highlightedMessageID
			m.highlightedMessageID = ""
			m.restyleMessageRows(previousID)
		}
		return m, nil
	}

	if frameMsg, ok := msg.(mouseMotionFrameMsg); ok {
		if frameMsg.generation == m.mouseMotionFrameGeneration && m.mouseMotion != nil {
			if x, y, known := m.mouseMotion.takeAndRelease(); known {
				m.applyMouseMotion(x, y)
			}
		}
		return m, nil
	}

	if clearMsg, ok := msg.(typingClearMsg); ok {
		if !m.typingTimerPending {
			return m, nil
		}
		remaining := time.Until(m.lastTypingAt.Add(typingClearDelay))
		if clearMsg.generation != m.typingGeneration && remaining > 0 {
			generation := m.typingGeneration
			return m, tea.Tick(remaining, func(time.Time) tea.Msg {
				return typingClearMsg{generation: generation}
			})
		}
		m.typingTimerPending = false
		if m.isTyping {
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
		m.lastMouseX = mouse.X
		m.lastMouseY = mouse.Y
		m.mousePositionKnown = true
		if m.mouseMotion != nil {
			m.mouseMotion.record(mouse.X, mouse.Y)
		}
		// A click takes priority over any pending passive-hover update. In
		// particular, don't make a header click wait for an old hover timer.
		m.refreshMessageHover()

		// A field edit owns the composer until Enter or Escape completes it.
		// Keep other panel clicks from closing the screen or firing actions.
		if m.themesOpen && m.themeEditField != "" && !m.inputSelectionContains(mouse.X, mouse.Y) {
			return m, nil
		}

		// The theme divider is a draggable one-cell splitter between the list
		// and details columns.
		if mouse.Button == tea.MouseLeft && m.themeBrowserDividerContains(mouse.X, mouse.Y) {
			m.themeDividerDragging = true
			m.setThemeBrowserDivider(mouse.X)
			return m, nil
		}

		if mouse.Button == tea.MouseLeft && m.themeEditField == "" && m.themeApplyButtonContains(mouse.X, mouse.Y) {
			m.applySelectedTheme()
			return m, nil
		}

		// Begin a textarea selection anywhere the composer is visible, including
		// while the splash, theme browser, or users panel is open.
		if mouse.Button == tea.MouseLeft && m.inputSelectionContains(mouse.X, mouse.Y) {
			localX, localY := m.inputSelectionCoordinates(mouse.X, mouse.Y)
			m.input.BeginSelection(localX, localY)
			m.inputSelecting = true
			return m, nil
		}

		// Footer actions occupy the final terminal row. Their hitboxes are
		// calculated by the same layout helper used to render the row.
		if m.statusBarVisible() && mouse.Button == tea.MouseLeft && mouse.Y == m.height-1 {
			_, targets := m.renderStatusBar(m.usersOpen, m.splashOpen && !m.themesOpen)
			if targets.hasApply && mouse.X >= targets.applyStart && mouse.X < targets.applyEnd {
				m.applySelectedTheme()
				return m, nil
			}
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
					m.quitting = true
					// main closes the socket after Program.Run returns. Closing it
					// here can race the packet reader and briefly render Disconnected.
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
			m.themesOpen = false
			m.usersScrollDragging = false
			m.usersScrollbarHoldDirection = 0
			m.usersScrollbarHoldGeneration++
			m.splashScrollDragging = false
			m.splashScrollbarHoldDirection = 0
			m.splashScrollbarHoldGeneration++
			return m, nil
		}
		if mouse.Button == tea.MouseLeft && mouse.Y == 0 && mouse.X >= 0 && mouse.X < lipgloss.Width("ZEALOT") {
			if m.themesOpen {
				m.themesOpen = false
				m.splashOpen = true
			} else if m.splashOpen {
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
			// Do not let a slow socket write hold up the UI response to the click.
			sendWhoCmd := func() tea.Msg {
				if m.client != nil {
					_ = m.client.send(Packet{Type: "who", Room: m.room})
				}
				return nil
			}
			if animationCmd := m.startUsersAnimation(); animationCmd != nil {
				return m, tea.Batch(sendWhoCmd, animationCmd)
			}
			return m, sendWhoCmd
		}
		if m.splashOpen && !m.themesOpen && mouse.Button == tea.MouseLeft && m.splashThemesButtonContains(mouse.X, mouse.Y) {
			m.openThemeBrowser()
			return m, nil
		}
		if m.themesOpen && mouse.Button == tea.MouseLeft {
			// While a theme field is being edited, keep clicks from changing
			// selection or triggering actions before the edit is confirmed.
			if m.themeEditField != "" {
				return m, nil
			}
			if m.themeCreateButtonContains(mouse.X, mouse.Y) {
				m.createFreshTheme()
				return m, nil
			}
			if m.themeCloneButtonContains(mouse.X, mouse.Y) {
				m.cloneSelectedTheme()
				return m, nil
			}
			if m.themeSaveButtonContains(mouse.X, mouse.Y) {
				m.saveSelectedThemeEdits()
				return m, nil
			}
			if m.themeApplyButtonContains(mouse.X, mouse.Y) {
				m.applySelectedTheme()
				return m, nil
			}
			if m.themeNameContains(mouse.X, mouse.Y) {
				if entry, ok := m.selectedThemeEntry(); ok && themeEntryEditable(entry) {
					m.beginThemeFieldEdit("name")
				}
				return m, nil
			}
			if field, ok := m.themeColorFieldAt(mouse.X, mouse.Y); ok {
				if entry, exists := m.selectedThemeEntry(); exists && themeEntryEditable(entry) {
					m.beginThemeFieldEdit(field)
				}
				return m, nil
			}
			if index, ok := m.themeBrowserListIndexAt(mouse.X, mouse.Y); ok {
				m.selectThemeEntry(index)
				return m, nil
			}
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
		if mouse.Button == tea.MouseLeft {
			if m.usersOpen {
				if username := m.userNameAt(mouse.X, mouse.Y); username != "" {
					return m, m.insertMention(username)
				}
			}
			if !m.usersOpen && !m.splashOpen {
				if username := m.messageUsernameAt(mouse.X, mouse.Y); username != "" {
					return m, m.insertMention(username)
				}
				if targetID := m.messageLinkAt(mouse.X, mouse.Y); targetID != "" {
					return m, m.navigateToMessage(targetID)
				}
				if id := m.messageIDAt(mouse.X, mouse.Y); id != "" {
					m.beginReply(id)
					return m, nil
				}
			}
		}
		// Mouse clicks outside the composer are fully handled above. Do not
		// feed panel/header clicks into the textarea or adaptive layout code.
		return m, nil

	case tea.MouseMotionMsg:
		if m.mouseMotion == nil {
			// The production filter owns the shared pointer state. This fallback
			// keeps direct model tests safe when they bypass program filtering.
			m.mouseMotion = &mouseMotionState{}
			m.mouseMotion.record(mouse.X, mouse.Y)
		}
		m.mouseMotionFrameGeneration++
		generation := m.mouseMotionFrameGeneration
		return m, tea.Tick(mouseMotionFrameInterval, func(time.Time) tea.Msg {
			return mouseMotionFrameMsg{generation: generation}
		})

	case tea.MouseReleaseMsg:
		// Apply the newest coalesced pointer position before ending a drag or
		// text selection, so the final release coordinate is not lost.
		if m.mouseMotion != nil {
			if x, y, known := m.mouseMotion.position(); known {
				m.applyMouseMotion(x, y)
			}
		}
		if m.inputSelecting {
			m.input.EndSelection()
			m.inputSelecting = false
		}
		m.themeDividerDragging = false
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
		if m.themesOpen {
			switch mouseMsg.Button {
			case tea.MouseWheelUp:
				m.moveThemeSelection(-3)
			case tea.MouseWheelDown:
				m.moveThemeSelection(3)
			}
			return m, nil
		}
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
		if m.themesOpen {
			switch keyMsg.String() {
			case "enter":
				if m.themeEditField != "" {
					m.finishThemeFieldEdit()
					return m, nil
				}
			case "esc", "escape":
				if m.themeEditField != "" {
					m.cancelThemeFieldEdit()
					return m, nil
				}
				m.themesOpen = false
				m.themeError = ""
				m.themeEditError = ""
				return m, nil
			case "ctrl+s":
				if m.themeEditField == "" {
					m.saveSelectedThemeEdits()
				}
				return m, nil
			case "up":
				if m.themeEditField == "" && m.input.Value() == "" {
					m.moveThemeSelection(-1)
					return m, nil
				}
			case "down":
				if m.themeEditField == "" && m.input.Value() == "" {
					m.moveThemeSelection(1)
					return m, nil
				}
			case "pgup":
				if m.themeEditField == "" && m.input.Value() == "" {
					m.moveThemeSelection(-m.themeVisibleRows())
					return m, nil
				}
			case "pgdown":
				if m.themeEditField == "" && m.input.Value() == "" {
					m.moveThemeSelection(m.themeVisibleRows())
					return m, nil
				}
			case "home":
				if m.themeEditField == "" && m.input.Value() == "" {
					m.selectThemeEntry(0)
					return m, nil
				}
			case "end":
				if m.themeEditField == "" && m.input.Value() == "" {
					m.selectThemeEntry(len(m.themeEntries) - 1)
					return m, nil
				}
			case "f2", "ctrl+enter":
				if m.themeEditField == "" {
					m.applySelectedTheme()
				}
				return m, nil
			case "ctrl+c", "ctrl+q":
				m.sendTyping(false)
				m.quitting = true
				return m, tea.Quit
			default:
				// Ordinary text and composer editing keys continue to the
				// textarea. Enter confirms a field edit and otherwise sends.
			}
		}
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
			m.quitting = true
			// main closes the socket after Program.Run returns; avoid racing the
			// packet reader and rendering a transient Disconnected state.
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
			m.refreshRenderedWindow()
			m.refreshMessageHover()
			return m, nil

		case "ctrl+end":
			if m.splashOpen {
				m.splashScroll = m.maxSplashScroll()
				return m, nil
			}
			m.scroll = 0
			m.refreshRenderedWindow()
			m.refreshMessageHover()
			return m, nil

		case "backspace":
			if m.replyTargetID != "" && !m.input.HasSelection() && m.input.Line() == 0 && m.input.Column() == 0 {
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
	previousInputHeight := m.input.Height()

	if !handledNewline {
		m.input, inputCmd = m.input.Update(msg)
	}
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && m.replyTargetID != "" && m.input.Value() == "" {
		if keyMsg.String() == "backspace" || keyMsg.String() == "delete" {
			m.cancelReply()
		}
	}
	if m.cfg.AdaptiveHeight && m.input.Height() != previousInputHeight {
		// Scroll limits depend on the available pane height, so recompute them
		// only when the adaptive textarea actually changes height.
		m.clampScroll()
		m.clampUsersScroll()
		m.clampSplashScroll()
	}

	// -------------------------------------------------------------------------
	// Typing detection
	// -------------------------------------------------------------------------

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		if m.connected && isTextInputKey(keyMsg) && m.themeEditField == "" {

			// Start blinking only when there is actual text.
			blinkCmd := m.startCursorBlink()

			if !m.isTyping {
				m.isTyping = true
				m.sendTyping(true)
			}

			// Consider the user to have stopped typing after
			// 1.2 seconds without another editing key.
			typingCmd := m.scheduleTypingClear()

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

const mouseMotionFrameInterval = time.Second / 60

// applyMouseMotion processes the latest pointer position. The motion filter
// calls this at most once per frame, regardless of how many terminal motion
// events arrived during that interval.
func (m *model) applyMouseMotion(x, y int) {
	m.lastMouseX = x
	m.lastMouseY = y
	m.mousePositionKnown = true

	if m.themesOpen && m.themeDividerDragging {
		m.setThemeBrowserDivider(x)
		return
	}
	if m.inputSelecting {
		localX, localY := m.inputSelectionCoordinates(x, y)
		m.input.ExtendSelection(localX, localY)
		m.refreshMessageHover()
		return
	}
	if m.splashOpen && m.splashScrollDragging {
		_, thumbHeight, trackHeight := m.splashScrollbarThumb()
		target := y - 3 - m.splashScrollDragOffset
		target = min(max(target, 0), max(0, trackHeight-thumbHeight))
		m.setSplashScrollFromThumbTop(target)
		return
	}
	if m.usersOpen && m.usersScrollDragging {
		_, thumbHeight, trackHeight := m.usersScrollbarThumb()
		target := y - 3 - m.usersScrollDragOffset
		target = min(max(target, 0), max(0, trackHeight-thumbHeight))
		m.setUsersScrollFromThumbTop(target)
		return
	}
	if m.scrollDragging && !m.usersOpen && !m.splashOpen {
		target := y - 3 - m.scrollDragOffset
		m.setScrollFromThumbTopForLineCount(target, len(m.renderMessageLines()))
		return
	}
	m.refreshMessageHover()
}

// scrollPage applies the same viewport movement for keyboard and footer clicks.
func (m *model) scrollPage(up bool) {
	if m.themesOpen {
		step := m.themeVisibleRows()
		if up { m.moveThemeSelection(-step) } else { m.moveThemeSelection(step) }
		return
	}
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

const typingClearDelay = 1200 * time.Millisecond

// scheduleTypingClear keeps only one outstanding timer for a continuous
// typing burst. Later keystrokes move the deadline; the first timer wakes once
// and, if necessary, arms a single replacement for the remaining quiet time.
func (m *model) scheduleTypingClear() tea.Cmd {
	m.typingGeneration++
	m.lastTypingAt = time.Now()
	if m.typingTimerPending {
		return nil
	}
	m.typingTimerPending = true
	generation := m.typingGeneration
	return tea.Tick(typingClearDelay, func(time.Time) tea.Msg {
		return typingClearMsg{generation: generation}
	})
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
