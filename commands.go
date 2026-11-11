package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

// sendCurrentMessage is shared by Enter and the clickable footer action.
func (m *model) sendCurrentMessage() {
	text := strings.TrimSpace(m.input.Value())
	if text == "" || !m.connected {
		return
	}

	sent := m.sendMessage(text, m.replyTargetID)
	m.input.Reset()
	m.input.CharLimit = maxMessage
	if sent && m.replyTargetID != "" {
		m.cancelReply()
	}
	m.clampScroll()
	m.clampUsersScroll()
	m.clampSplashScroll()
	m.stopCursorBlink()

	if m.isTyping {
		m.sendTyping(false)
		m.isTyping = false
		m.typingWhisperTarget = ""
	}
}

func (m *model) sendMessage(text string, replyTo string) bool {
	trimmed := strings.TrimSpace(text)
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return false
	}
	// Replying to a whisper always stays private, even when the draft
	// resembles a slash command. Reuse the selected reply target rather
	// than creating a separate composer or interaction state.
	if replyTo != "" {
		if original, ok := m.messageByID[replyTo]; ok && original.Type == "whisper_result" {
			recipient := original.User.Name
			if strings.EqualFold(recipient, m.user.Name) {
				recipient = original.WhisperTo
			}
			if recipient == "" || len([]rune(trimmed)) > maxMessage {
				m.appendMessage(Packet{Type: "system", Text: "Cannot send whisper reply: invalid recipient or message length.", Time: time.Now().UTC()})
				return false
			}
			_ = m.client.send(Packet{
				Type: "whisper", Room: m.room, WhisperTo: "@"+recipient,
				Text: trimmed, ReplyTo: replyTo,
			})
			return true
		}
	}
	switch strings.ToLower(fields[0]) {
	case "/long", "/spoiler":
		body := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
		if body == "" {
			m.appendMessage(Packet{Type: "system", Text: "Usage: " + strings.ToLower(fields[0]) + " message", Time: time.Now().UTC()})
			return false
		}
		longMessage := strings.EqualFold(fields[0], "/long")
		limit := maxMessage
		if longMessage {
			limit = maxLongMessage
		}
		if len([]rune(body)) > limit {
			m.appendMessage(Packet{Type: "system", Text: fmt.Sprintf("Message exceeds the %d-character limit.", limit), Time: time.Now().UTC()})
			return false
		}
		_ = m.client.send(Packet{
			Type: "message", ReplyTo: replyTo, Room: m.room, User: m.user,
			Text: body, Long: longMessage, Spoiler: !longMessage,
		})
		return true
	case "/whisper":
		// Split the message only twice: the body may contain arbitrary spaces,
		// punctuation and line breaks, but the recipient must be a single name.
		parts := strings.Fields(trimmed)
		if len(parts) < 3 || !strings.HasPrefix(parts[1], "@") || len(parts[1]) < 2 {
			m.appendMessage(Packet{Type: "system", Text: "Usage: /whisper @user message", Time: time.Now().UTC()})
			return false
		}
		body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0])), parts[1]))
		if body == "" || len([]rune(body)) > maxMessage {
			m.appendMessage(Packet{Type: "system", Text: fmt.Sprintf("Whispers must contain 1–%d characters. Usage: /whisper @user message", maxMessage), Time: time.Now().UTC()})
			return false
		}
		_ = m.client.send(Packet{Type: "whisper", Room: m.room, WhisperTo: parts[1], Text: body})
		return true
	case "/coinflip":
		if len(fields) != 1 {
			m.appendMessage(Packet{Type: "system", Text: "Usage: /coinflip", Time: time.Now().UTC()})
			return false
		}
		_ = m.client.send(Packet{Type: "coinflip", Room: m.room})
		return true
	case "/random":
		if len(fields) != 2 {
			m.appendMessage(Packet{Type: "system", Text: "Usage: /random MIN-MAX", Time: time.Now().UTC()})
			return false
		}
		if _, _, err := parseRandomRange(fields[1]); err != nil {
			m.appendMessage(Packet{Type: "system", Text: err.Error(), Time: time.Now().UTC()})
			return false
		}
		_ = m.client.send(Packet{Type: "random", Room: m.room, Text: fields[1]})
		return true
	case "/roll":
		if len(fields) != 2 {
			m.appendMessage(Packet{Type: "system", Text: "Usage: /roll NdM (e.g. /roll 2d6)", Time: time.Now().UTC()})
			return false
		}
		if _, _, err := parseDiceNotation(fields[1]); err != nil {
			m.appendMessage(Packet{Type: "system", Text: err.Error(), Time: time.Now().UTC()})
			return false
		}
		_ = m.client.send(Packet{Type: "roll", Room: m.room, Text: fields[1]})
		return true
	case "/fortune":
		if len(fields) > 2 || (len(fields) == 2 && (!strings.HasPrefix(fields[1], "@") || len(fields[1]) == 1)) {
			m.appendMessage(Packet{Type: "system", Text: "Usage: /fortune [@user]", Time: time.Now().UTC()})
			return false
		}
		target := ""
		if len(fields) == 2 {
			target = fields[1]
		}
		_ = m.client.send(Packet{Type: "fortune", Room: m.room, Text: target})
		return true
	case "/rb":
		body := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
		if body == "" {
			m.appendMessage(Packet{Type: "system", Text: "Usage: /rb message", Time: time.Now().UTC()})
			return false
		}
		packet := Packet{
			Type:         "message",
			ReplyTo:      replyTo,
			Room:         m.room,
			User:         m.user,
			Text:         body,
			Rainbow:      true,
			RainbowPhase: rand.Float64(),
		}
		_ = m.client.send(packet)
		return true
	case "/xkeyscore":
		var variant int
		if len(fields) == 2 {
			if requested, err := strconv.Atoi(fields[1]); err == nil && requested >= 1 && requested <= 3 {
				variant = requested
			}
		}
		if variant == 0 {
			variant = rand.Intn(3) + 1
		}
		packet := Packet{
			Type:         "message",
			ReplyTo:      replyTo,
			Room:         m.room,
			User:         m.user,
			Text:         "XKEYSCORE",
			XKeyscore:    variant,
			Rainbow:      m.rainbowMode,
			RainbowPhase: rand.Float64(),
		}
		_ = m.client.send(packet)
		return true
	case "/rainbow":
		if len(fields) != 1 {
			m.appendMessage(Packet{Type: "system", Text: "Usage: /rainbow", Time: time.Now()})
			return false
		}
		m.rainbowMode = !m.rainbowMode
		status := "enabled"
		if !m.rainbowMode {
			status = "disabled"
		}
		m.appendMessage(Packet{Type: "system", Text: "Rainbow mode " + status + " for your messages.", Time: time.Now().UTC()})
		return false
	case "/who":
		if len(fields) != 1 {
			m.appendMessage(Packet{Type: "system", Text: "Usage: /who", Time: time.Now()})
			return false
		}
		// The room roster is maintained continuously from server broadcasts,
		// so /who can display the current snapshot without requesting another
		// "users" packet (which is also used for typing/join/leave updates).
		text := fmt.Sprintf("Connected users (%d):", len(m.onlineUsers))
		// Leave the permanent typing-indicator row out of the available chat
		// height. Include the heading when deciding whether the vertical list
		// will fit; otherwise use a compact paragraph that wraps naturally.
		availableRows := max(1, m.messageHeight()-1)
		if len(m.onlineUsers)+1 <= availableRows {
			for _, user := range m.onlineUsers {
				text += "\n  • " + user.Name
			}
		} else if len(m.onlineUsers) > 0 {
			names := make([]string, 0, len(m.onlineUsers))
			for _, user := range m.onlineUsers {
				names = append(names, user.Name)
			}
			text += " " + strings.Join(names, ", ")
		}
		m.appendMessage(Packet{
			Type: "system",
			Text: text,
			Time: time.Now().UTC(),
		})
		return false
	case "/history":
		if len(fields) != 1 {
			m.appendMessage(Packet{Type: "system", Text: "Usage: /history", Time: time.Now()})
			return false
		}
		_ = m.client.send(Packet{Type: "history_status", Room: m.room})
		return false
	case "/rename":
		if len(fields) != 2 {
			m.appendMessage(Packet{Type: "system", Text: "Usage: /rename name", Time: time.Now()})
			return false
		}
		// Stop typing under the old identity before asking the server to rename us.
		if m.isTyping {
			m.isTyping = false
			m.typingGeneration++ // invalidate any pending typing timeout
			m.sendTyping(false)
		}
		old := m.cfg.Name
		m.cfg.Name = fields[1]
		if err := saveConfigName(fields[1]); err != nil {
			m.cfg.Name = old
			m.appendMessage(Packet{Type: "system", Text: "Could not save config: " + err.Error(), Time: time.Now()})
			return false
		}
		_ = m.client.send(Packet{Type: "rename", Room: m.room, User: User{Name: fields[1], Color: m.user.Color}})
		return false
	case "/color":
		if len(fields) != 2 || !validUserColor(fields[1]) {
			m.appendMessage(Packet{Type: "system", Text: "Usage: /color #RRGGBB (or an ANSI color number 0–255)", Time: time.Now()})
			return false
		}
		old := m.cfg.UserColor
		m.cfg.UserColor = fields[1]
		m.user.Color = fields[1]
		if err := saveConfigUserColor(fields[1]); err != nil {
			m.cfg.UserColor = old
			m.user.Color = old
			m.appendMessage(Packet{Type: "system", Text: "Could not save config: " + err.Error(), Time: time.Now()})
			return false
		}
		_ = m.client.send(Packet{Type: "color", Room: m.room, User: m.user})
		return false
	case "/me":
		body := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
		if body == "" {
			m.appendMessage(Packet{Type: "system", Text: "Usage: /me action", Time: time.Now()})
			return false
		}
		_ = m.client.send(Packet{Type: "me", Room: m.room, User: m.user, Text: body})
		return false
	}
	packet := Packet{Type: "message", ReplyTo: replyTo, Room: m.room, User: m.user, Text: text}
	if m.rainbowMode {
		packet.Rainbow = true
		packet.RainbowPhase = rand.Float64()
	}
	_ = m.client.send(packet)
	return true
}

// splashRows builds the splash content at the current terminal width. The
// fixed-width block lettering spans 74 columns and is centered in the
