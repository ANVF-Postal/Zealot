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
	if sent && m.replyTargetID != "" {
		m.cancelReply()
	}
	m.clampScroll()
	m.clampUsersScroll()
	m.clampSplashScroll()
	m.stopCursorBlink()

	if m.isTyping {
		m.isTyping = false
		m.sendTyping(false)
	}
}

func (m *model) sendMessage(text string, replyTo string) bool {
	trimmed := strings.TrimSpace(text)
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return false
	}
	switch strings.ToLower(fields[0]) {
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
			return
		}
		m.rainbowMode = !m.rainbowMode
		status := "enabled"
		if !m.rainbowMode {
			status = "disabled"
		}
		m.appendMessage(Packet{Type: "system", Text: "Rainbow mode " + status + " for your messages.", Time: time.Now().UTC()})
		return
	case "/who":
		if len(fields) != 1 {
			m.appendMessage(Packet{Type: "system", Text: "Usage: /who", Time: time.Now()})
			return
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
		return
	case "/history":
		if len(fields) != 1 {
			m.appendMessage(Packet{Type: "system", Text: "Usage: /history", Time: time.Now()})
			return
		}
		_ = m.client.send(Packet{Type: "history_status", Room: m.room})
		return
	case "/rename":
		if len(fields) != 2 {
			m.appendMessage(Packet{Type: "system", Text: "Usage: /rename name", Time: time.Now()})
			return
		}
		// Stop typing under the old identity before asking the server to rename us.
		if m.isTyping {
			m.isTyping = false
			m.typingGeneration++ // invalidate any pending typing timeout
			m.sendTyping(false)
		}
		old := m.cfg.Name
		m.cfg.Name = fields[1]
		if err := saveConfig(m.cfg); err != nil {
			m.cfg.Name = old
			m.appendMessage(Packet{Type: "system", Text: "Could not save config: " + err.Error(), Time: time.Now()})
			return
		}
		_ = m.client.send(Packet{Type: "rename", Room: m.room, User: User{Name: fields[1], Color: m.user.Color}})
		return
	case "/color":
		if len(fields) != 2 || !validUserColor(fields[1]) {
			m.appendMessage(Packet{Type: "system", Text: "Usage: /color #RRGGBB (or an ANSI color number 0–255)", Time: time.Now()})
			return
		}
		old := m.cfg.UserColor
		m.cfg.UserColor = fields[1]
		m.user.Color = fields[1]
		if err := saveConfig(m.cfg); err != nil {
			m.cfg.UserColor = old
			m.user.Color = old
			m.appendMessage(Packet{Type: "system", Text: "Could not save config: " + err.Error(), Time: time.Now()})
			return
		}
		_ = m.client.send(Packet{Type: "color", Room: m.room, User: m.user})
		return
	case "/me":
		body := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
		if body == "" {
			m.appendMessage(Packet{Type: "system", Text: "Usage: /me action", Time: time.Now()})
			return
		}
		_ = m.client.send(Packet{Type: "me", Room: m.room, User: m.user, Text: body})
		return
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
