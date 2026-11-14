package main

import (
	"fmt"
	"strings"
	"time"
)

// sendWhisper delivers a private message exclusively to the requester and a
// currently connected room member. It deliberately never calls broadcast and
// never appends to room history. The authenticated connection, not the request
// packet, supplies the sender's identity and room.
func (s *server) sendWhisper(roomName string, sender *serverClient, target, body, replyTo string) error {
	body = strings.TrimSpace(body)
	if !strings.HasPrefix(target, "@") || len(target) < 2 || strings.TrimSpace(target[1:]) == "" {
		return fmt.Errorf("Usage: /whisper @user message")
	}
	if body == "" || len([]rune(body)) > maxMessage {
		return fmt.Errorf("Whispers must contain 1–%d characters. Usage: /whisper @user message", maxMessage)
	}
	if strings.ContainsAny(target, " \r\n\t") {
		return fmt.Errorf("Usage: /whisper @user message")
	}

	s.mu.Lock()
	room := s.rooms[roomName]
	var recipient *serverClient
	if room != nil {
		if room.clients[sender.id] != sender {
			s.mu.Unlock()
			return fmt.Errorf("Whisper failed: sender is no longer connected")
		}
		for _, member := range room.clients {
			if strings.EqualFold(member.user.Name, target[1:]) {
				recipient = member
				break
			}
		}
	}
	if recipient == nil {
		s.mu.Unlock()
		return fmt.Errorf("Whisper failed: @%s is not online in this room", terminalSafeText(target[1:]))
	}

	result := Packet{
		ID: nextMessageID(), Type: "whisper_result", Room: roomName,
		User: sender.user, WhisperTo: recipient.user.Name, Text: body, ReplyTo: replyTo,
		Time: time.Now().UTC(),
	}
	// Snapshot the authorized recipients under the room lock, then send
	// only to those connections. No room-wide fan-out or history storage.
	s.mu.Unlock()
	sender.send(result)
	if recipient != sender {
		recipient.send(result)
	}
	return nil
}

// setWhisperTyping delivers transient typing state only to the resolved
// recipient. The daemon uses connection identities and never trusts the
// sender/room fields in a client-supplied packet.
func (s *server) setWhisperTyping(roomName string, sender *serverClient, requested string, active bool) {
	s.mu.Lock()
	room := s.rooms[roomName]
	if room == nil || room.clients[sender.id] != sender {
		s.mu.Unlock()
		return
	}
	var recipient *serverClient
	if active && strings.HasPrefix(requested, "@") && !strings.ContainsAny(requested, " \t\r\n") {
		for _, member := range room.clients {
			if member != sender && strings.EqualFold(member.user.Name, requested[1:]) {
				recipient = member
				break
			}
		}
	}
	previous := sender.whisperTarget
	var oldRecipient *serverClient
	if previous != "" {
		for _, member := range room.clients {
			if strings.EqualFold(member.user.Name, previous) {
				oldRecipient = member
				break
			}
		}
	}
	next := ""
	if recipient != nil {
		next = recipient.user.Name
	}
	sender.whisperTarget = next
	wasPublic := sender.typing
	sender.typing = false
	user := sender.user
	s.mu.Unlock()

	if oldRecipient != nil && (recipient != oldRecipient || !active) {
		oldRecipient.send(Packet{Type: "whisper_typing", Room: roomName, User: user, Typing: false})
	}
	if recipient != nil && recipient != oldRecipient {
		recipient.send(Packet{Type: "whisper_typing", Room: roomName, User: user, Typing: true})
	}
	if wasPublic {
		s.broadcast(roomName, Packet{Type: "typing", Room: roomName, User: user, Typing: false})
		s.broadcastRoster(roomName)
	}
}
