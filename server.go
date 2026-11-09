package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// -----------------------------------------------------------------------------
// Server
// -----------------------------------------------------------------------------

var messageSequence atomic.Uint64

func nextMessageID() string {
	return fmt.Sprintf("%x-%x", time.Now().UnixNano(), messageSequence.Add(1))
}

type serverClient struct {
	enc       *json.Encoder
	writer    *bufio.Writer
	mu        sync.Mutex
	id        string
	room      string
	user      User
	typing    bool
	ephemeral bool
}

type roomState struct {
	messages       []Packet
	clients        map[string]*serverClient
	historyEnabled bool
}

type server struct {
	mu    sync.Mutex
	rooms map[string]*roomState
}

func newServer() *server {
	return &server{rooms: make(map[string]*roomState)}
}

func (s *server) room(name string, historyEnabled bool) *roomState {
	r := &roomState{
		clients:        make(map[string]*serverClient),
		historyEnabled: historyEnabled,
	}
	s.rooms[name] = r
	return r
}

func (s *server) broadcast(
	roomName string,
	packet Packet,
) {
	s.mu.Lock()

	room := s.rooms[roomName]

	var clients []*serverClient

	if room != nil {
		for _, c := range room.clients {
			clients = append(clients, c)
		}
	}

	s.mu.Unlock()

	for _, c := range clients {
		c.send(packet)
	}
}

// broadcastRoster sends a current, alphabetized room roster to every client.
func (s *server) broadcastRoster(roomName string) {
	s.mu.Lock()
	room := s.rooms[roomName]
	users := make([]User, 0)
	if room != nil {
		for _, client := range room.clients {
			user := client.user
			user.Typing = client.typing
			users = append(users, user)
		}
	}
	s.mu.Unlock()

	sort.Slice(users, func(i, j int) bool {
		return strings.ToLower(users[i].Name) < strings.ToLower(users[j].Name)
	})
	s.broadcast(roomName, Packet{Type: "users", Room: roomName, Users: users})
}

func (c *serverClient) send(packet Packet) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.enc.Encode(packet); err == nil && c.writer != nil {
		_ = c.writer.Flush()
	}
}

func (s *server) systemMessage(
	roomName string,
	text string,
) {
	packet := Packet{
		ID:   nextMessageID(),
		Type: "system",
		Room: roomName,
		Text: text,
		Time: time.Now().UTC(),
	}
	s.mu.Lock()
	if room := s.rooms[roomName]; room != nil && room.historyEnabled {
		room.messages = append(room.messages, packet)
	}
	s.mu.Unlock()
	s.broadcast(roomName, packet)
}

func (s *server) handle(conn net.Conn) {
	defer conn.Close()

	decoder := json.NewDecoder(
		bufio.NewReader(conn),
	)

	writer := bufio.NewWriterSize(conn, 64*1024)
	encoder := json.NewEncoder(writer)

	var hello Packet

	if err := decoder.Decode(&hello); err != nil {
		return
	}

	if hello.Type != "hello" {
		return
	}

	roomName := cleanRoom(hello.Room)

	if roomName == "" {
		roomName = "lobby"
	}

	user := hello.User

	if strings.TrimSpace(user.Name) == "" {
		user.Name = "Anonymous"
	}

	client := &serverClient{
		enc:    encoder,
		writer: writer,
		id: fmt.Sprintf(
			"%d-%p",
			time.Now().UnixNano(),
			conn,
		),
		room:      roomName,
		user:      user,
		ephemeral: hello.Ephemeral,
	}

	s.mu.Lock()
	room, exists := s.rooms[roomName]
	// --add-history only establishes policy when creating a room. A normal
	// join always inherits the existing room's policy; it must not need to
	// repeat the creation flag. Passing the flag to an existing room is
	// rejected so nobody can use it to alter that room's history setting.
	if exists && hello.AddHistory {
		existingPolicy := "disabled"
		if room.historyEnabled {
			existingPolicy = "enabled"
		}
		_ = encoder.Encode(Packet{
			Type: "history_mismatch",
			Text: fmt.Sprintf("This room already exists with history %s. Remove the --add-history flag and relaunch to join it.", existingPolicy),
		})
		_ = writer.Flush()
		s.mu.Unlock()
		return
	}
	if !exists {
		room = s.room(roomName, hello.AddHistory)
	}
	user.Name = uniqueUserName(room, user.Name)
	client.user = user
	room.clients[client.id] = client
	var history []Packet
	historyEnabled := room.historyEnabled
	if historyEnabled {
		history = append([]Packet(nil), room.messages...)
	}

	// Hold this client's write lock before releasing the server lock. Any
	// concurrent broadcast will wait behind the welcome/history sequence,
	// while other rooms remain free to proceed.
	client.mu.Lock()
	s.mu.Unlock()

	writeErr := client.enc.Encode(Packet{
		Type:       "welcome",
		Room:       roomName,
		User:       client.user,
		AddHistory: historyEnabled,
	})
	if writeErr == nil {
		for _, message := range history {
			if writeErr = client.enc.Encode(message); writeErr != nil {
				break
			}
		}
	}
	if writeErr == nil && historyEnabled {
		writeErr = client.enc.Encode(Packet{
			Type: "system",
			Room: roomName,
			Text: "History is enabled for this room.",
			Time: time.Now().UTC(),
		})
	}
	if writeErr == nil {
		// Mark the end of replay so history does not trigger notifications.
		writeErr = client.enc.Encode(Packet{Type: "history_end"})
	}

	if writeErr == nil {
		s.mu.Lock()
		onlineCount := len(room.clients)
		s.mu.Unlock()

		writeErr = client.enc.Encode(Packet{
			Type: "online",
			Text: strconv.Itoa(onlineCount),
		})
	}
	if writeErr == nil {
		// History is a replay burst. Flush it as a buffered stream rather than
		// issuing one socket write for every JSON packet.
		writeErr = client.writer.Flush()
	}

	client.mu.Unlock()
	if writeErr != nil {
		s.removeClient(client)
		return
	}

	// Broadcast the authoritative room state so clients already in the room
	// receive the same daemon-controlled setting as the newly joined client.
	s.broadcast(roomName, Packet{
		Type:       "history_state",
		Room:       roomName,
		AddHistory: historyEnabled,
	})

	if !client.ephemeral {
		s.systemMessage(
			roomName,
			fmt.Sprintf("%s joined the room.", user.Name),
		)
	}
	s.mu.Lock()
	onlineCount := len(s.rooms[roomName].clients)
	s.mu.Unlock()

	s.broadcast(roomName, Packet{
		Type: "online",
		Text: strconv.Itoa(onlineCount),
	})
	s.broadcastRoster(roomName)

	for {
		var packet Packet

		if err := decoder.Decode(&packet); err != nil {
			s.removeClient(client)
			return
		}

		switch packet.Type {
		case "coinflip":
			result, err := newCoinflipResult(roomName, client.user)
			if err != nil {
				client.send(Packet{Type: "system", Room: roomName, Text: "Coin flip failed: could not obtain secure randomness.", Time: time.Now().UTC()})
				continue
			}
			s.broadcastGameResult(roomName, client, result)

		case "random":
			result, err := newRandomResult(roomName, client.user, packet.Text)
			if err != nil {
				client.send(Packet{Type: "system", Room: roomName, Text: err.Error(), Time: time.Now().UTC()})
				continue
			}
			s.broadcastGameResult(roomName, client, result)


		case "letter":
			// Letters are one-shot, multiline messages sent by stdin/pipes.
			// Unlike regular chat, their whitespace and line breaks are meaningful,
			// and they are deliberately not added to room history.
			text := strings.TrimRight(packet.Text, "\r\n")
			if strings.TrimSpace(text) == "" {
				continue
			}
			if len([]rune(text)) > 100000 {
				runes := []rune(text)
				text = string(runes[:100000])
			}
			s.broadcast(roomName, Packet{
				ID:   nextMessageID(),
				Type: "letter",
				Room: roomName,
				User: user,
				Text: text,
				Time: time.Now().UTC(),
			})
			// Confirm delivery to the one-shot sender. Without this handshake,
			// the CLI can close immediately after writing and cannot distinguish
			// successful delivery from an older daemon that ignores "letter".
			client.send(Packet{Type: "letter_ack", Room: roomName})

		case "message":
			text := strings.TrimSpace(packet.Text)

			if text == "" {
				continue
			}

			if len([]rune(text)) > maxMessage {
				runes := []rune(text)
				text = string(runes[:maxMessage])
			}

			message := Packet{
				ID:          nextMessageID(),
				ReplyTo:     packet.ReplyTo,
				Type:        "message",
				Room:        roomName,
				User:        user,
				Text:        text,
				Time:        time.Now().UTC(),
				Rainbow:      packet.Rainbow,
				RainbowPhase: packet.RainbowPhase,
				XKeyscore:    packet.XKeyscore,
			}

			s.mu.Lock()

			room := s.rooms[roomName]
			if room != nil {
				roomUsernames := make([]string, 0, len(room.clients))
				for _, connected := range room.clients {
					roomUsernames = append(roomUsernames, connected.user.Name)
				}
				message.Mentions = extractMentionNames(text, roomUsernames)
			}
			if room != nil && room.historyEnabled {
				room.messages = append(room.messages, message)
			}

			client.typing = false

			s.mu.Unlock()

			s.broadcast(roomName, message)

			s.broadcast(roomName, Packet{
				Type:   "typing",
				Room:   roomName,
				User:   user,
				Typing: false,
			})
			s.broadcastRoster(roomName)

		case "rename":
			requested := strings.TrimSpace(packet.User.Name)
			if requested == "" || len([]rune(requested)) > 32 || strings.ContainsAny(requested, "\r\n\t") {
				client.send(Packet{Type: "system", Room: roomName, Text: "Usage: /rename name (1–32 characters).", Time: time.Now().UTC()})
				continue
			}
			oldName := client.user.Name
			s.mu.Lock()
			room := s.rooms[roomName]
			if room != nil {
				delete(room.clients, client.id)
			}
			if room != nil {
				requested = uniqueUserName(room, requested)
				room.clients[client.id] = client
			}
			// Clear any typing state under the old name before changing identity.
			wasTyping := client.typing
			client.typing = false
			client.user.Name = requested
			user = client.user
			s.mu.Unlock()
			if wasTyping {
				s.broadcast(roomName, Packet{Type: "typing", Room: roomName, User: User{Name: oldName}, Typing: false})
			}
			s.systemMessage(roomName, fmt.Sprintf("%s is now %s", oldName, requested))
			client.send(Packet{Type: "identity", Room: roomName, User: client.user})
			s.broadcastRoster(roomName)

		case "color":
			color := strings.TrimSpace(packet.User.Color)
			if !validUserColor(color) {
				client.send(Packet{Type: "system", Room: roomName, Text: "Usage: /color #RRGGBB (or an ANSI color number 0–255).", Time: time.Now().UTC()})
				continue
			}
			s.mu.Lock()
			client.user.Color = color
			user = client.user
			s.mu.Unlock()
			client.send(Packet{Type: "identity", Room: roomName, User: client.user})
			s.systemMessage(roomName, fmt.Sprintf("%s changed their color.", user.Name))
			s.broadcastRoster(roomName)

		case "me":
			text := strings.TrimSpace(packet.Text)
			if text == "" {
				continue
			}
			if len([]rune(text)) > maxMessage {
				text = string([]rune(text)[:maxMessage])
			}
			action := Packet{ID: nextMessageID(), ReplyTo: packet.ReplyTo, Type: "me", Room: roomName, User: user, Text: text, Time: time.Now().UTC()}
			s.mu.Lock()
			room := s.rooms[roomName]
			if room != nil && room.historyEnabled {
				room.messages = append(room.messages, action)
			}
			client.typing = false
			s.mu.Unlock()
			s.broadcast(roomName, action)
			s.broadcast(roomName, Packet{Type: "typing", Room: roomName, User: user, Typing: false})
			s.broadcastRoster(roomName)

		case "typing":
			typing := packet.Typing

			s.mu.Lock()
			client.typing = typing
			s.mu.Unlock()

			s.broadcast(roomName, Packet{
				Type:   "typing",
				Room:   roomName,
				User:   user,
				Typing: typing,
			})
			s.broadcastRoster(roomName)

		case "who":
			// Return a structured roster snapshot only to the requesting client.
			s.mu.Lock()
			room := s.rooms[roomName]
			users := make([]User, 0)
			if room != nil {
				for _, connected := range room.clients {
					user := connected.user
					user.Typing = connected.typing
					users = append(users, user)
				}
			}
			s.mu.Unlock()
			sort.Slice(users, func(i, j int) bool {
				return strings.ToLower(users[i].Name) < strings.ToLower(users[j].Name)
			})
			client.send(Packet{Type: "users", Room: roomName, Users: users})

		case "history_status":
			s.mu.Lock()
			room := s.rooms[roomName]
			enabled := room != nil && room.historyEnabled
			s.mu.Unlock()
			status := "History is disabled for this room."
			if enabled {
				status = "History is enabled for this room (in memory while the room remains active)."
			}
			client.send(Packet{Type: "system", Room: roomName, Text: status, Time: time.Now().UTC()})
		}
	}
}

func (s *server) removeClient(
	client *serverClient,
) {
	s.mu.Lock()

	room := s.rooms[client.room]

	if room == nil {
		s.mu.Unlock()
		return
	}

	delete(room.clients, client.id)

	remaining := len(room.clients)

	if remaining == 0 {
		// Destroy the room and its entire message history.
		delete(s.rooms, client.room)
	}

	s.mu.Unlock()

	if remaining > 0 {
		if client.typing {
			s.broadcast(client.room, Packet{
				Type:   "typing",
				Room:   client.room,
				User:   client.user,
				Typing: false,
			})
		}

		if !client.ephemeral {
			s.systemMessage(
				client.room,
				fmt.Sprintf("%s left the room.", client.user.Name),
			)
		}
		s.mu.Lock()
		room := s.rooms[client.room]
		onlineCount := 0
		if room != nil {
			onlineCount = len(room.clients)
		}
		s.mu.Unlock()

		s.broadcast(client.room, Packet{
			Type: "online",
			Text: strconv.Itoa(onlineCount),
		})
		s.broadcastRoster(client.room)

	}
}

// uniqueUserName preserves the requested name when available, otherwise adding
// the first free numeric suffix (Name2, Name3, ...). Name matching is
// case-insensitive so visually identical names cannot collide by case alone.
func uniqueUserName(room *roomState, requested string) string {
	base := strings.TrimSpace(requested)
	if base == "" {
		base = "Anonymous"
	}

	used := make(map[string]struct{}, len(room.clients))
	for _, client := range room.clients {
		used[strings.ToLower(client.user.Name)] = struct{}{}
	}
	if _, exists := used[strings.ToLower(base)]; !exists {
		return base
	}

	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s%d", base, suffix)
		if _, exists := used[strings.ToLower(candidate)]; !exists {
			return candidate
		}
	}
}

func listenUnixSocket() (net.Listener, error) {
	listener, err := net.Listen("unix", socketPath)
	if err == nil {
		return listener, nil
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		return nil, err
	}

	// A live daemon owns this socket. Never unlink it: doing so would leave
	// existing clients attached to an orphaned listener and split new users
	// into a second server instance.
	for i := 0; i < 10; i++ {
		conn, dialErr := net.DialTimeout("unix", socketPath, 150*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			return nil, fmt.Errorf("Zealot daemon is already running")
		}
		time.Sleep(25 * time.Millisecond)
	}

	info, statErr := os.Lstat(socketPath)
	if statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return net.Listen("unix", socketPath)
		}
		return nil, statErr
	}
	if info.Mode()&os.ModeSocket == 0 {
		return nil, fmt.Errorf("%s exists and is not a Unix socket", socketPath)
	}
	if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return net.Listen("unix", socketPath)
}

func runServer() error {
	listener, err := listenUnixSocket()
	if err != nil {
		return err
	}
	socketInfo, err := os.Lstat(socketPath)
	if err != nil {
		_ = listener.Close()
		return err
	}

	defer func() {
		_ = listener.Close()
		// Only remove the socket this daemon created; never unlink a path that
		// another process may have replaced during shutdown.
		if currentInfo, statErr := os.Lstat(socketPath); statErr == nil && os.SameFile(socketInfo, currentInfo) {
			_ = os.Remove(socketPath)
		}
	}()

	// Local Unix socket only.
	//
	// No TCP socket is opened, so Zealot does not listen on the LAN.
	if err := os.Chmod(socketPath, 0666); err != nil {
		return err
	}

	s := newServer()

	unixListener, ok := listener.(*net.UnixListener)

	if !ok {
		return fmt.Errorf("unexpected listener type")
	}

	for {
		if err := unixListener.SetDeadline(
			time.Now().Add(1 * time.Second),
		); err != nil {
			return err
		}

		conn, err := unixListener.Accept()

		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				s.mu.Lock()
				empty := len(s.rooms) == 0
				s.mu.Unlock()

				if empty {
					return nil
				}

				continue
			}

			return err
		}

		go s.handle(conn)
	}
}

func cleanRoom(room string) string {
	room = strings.TrimSpace(room)

	if len(room) > 64 {
		room = room[:64]
	}

	var b strings.Builder

	for _, r := range room {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' ||
			r == '_' {
			b.WriteRune(r)
		}
	}

	return b.String()
}
