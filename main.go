package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	socketPath = "/tmp/zealot.sock"
	maxMessage = 4000
)

// -----------------------------------------------------------------------------
// Configuration
// -----------------------------------------------------------------------------

type Config struct {
	Name        string
	UserColor   string
	Bell        bool
	InputHeight int
	Background  string
	TextColor   string
	BorderColor string
	AccentColor string
}

func defaultConfig() Config {
	return Config{
		Name:        "Anonymous",
		Bell:        true,
		InputHeight: 1,
		UserColor:   "#7CFFCB",
		Background:  "#111318",
		TextColor:   "#E6E6E6",
		BorderColor: "#444B5A",
		AccentColor: "#7C5CFC",
	}
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}

	return filepath.Join(dir, "zealot", "config.toml")
}

func loadConfig() (Config, error) {
	cfg := defaultConfig()
	path := configPath()

	data, err := os.ReadFile(path)

	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return cfg, err
		}

		if err := os.WriteFile(
			path,
			[]byte(defaultConfigFile()),
			0600,
		); err != nil {
			return cfg, err
		}

		return cfg, nil
	}

	if err != nil {
		return cfg, err
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"`)

		switch key {
		case "name":
			cfg.Name = value
		case "user_color":
			cfg.UserColor = value
		case "background":
			cfg.Background = value
		case "text_color":
			cfg.TextColor = value
		case "border_color":
			cfg.BorderColor = value
		case "accent_color":
			cfg.AccentColor = value
		case "bell":
			if enabled, err := strconv.ParseBool(value); err == nil {
				cfg.Bell = enabled
			}
		case "input_height":
			if height, err := strconv.Atoi(value); err == nil {
				cfg.InputHeight = height
			}
		}
	}

	if cfg.InputHeight < 1 {
		cfg.InputHeight = 1
	}
	if cfg.InputHeight > 8 {
		cfg.InputHeight = 8
	}

	if strings.TrimSpace(cfg.Name) == "" {
		cfg.Name = "Anonymous"
	}

	return cfg, nil
}

func defaultConfigFile() string {
	return `# Zealot configuration
#
# Colors may be ANSI colors ("12"), 256-color numbers,
# or hexadecimal colors ("#7CFFCB").

name = "Anonymous"

user_color = "#7CFFCB"

bell = true

# Visible text rows in the composer (1-8). Longer messages wrap onto new rows.
input_height = 1

background = "#111318"
text_color = "#E6E6E6"
border_color = "#444B5A"
accent_color = "#7C5CFC"
`
}

func saveConfig(cfg Config) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	content := fmt.Sprintf(`# Zealot configuration
# Colors may be ANSI colors ("12"), 256-color numbers, or hexadecimal colors ("#7CFFCB").

name = %q
user_color = %q
bell = %t
input_height = %d
background = %q
text_color = %q
border_color = %q
accent_color = %q
`, cfg.Name, cfg.UserColor, cfg.Bell, cfg.InputHeight, cfg.Background, cfg.TextColor, cfg.BorderColor, cfg.AccentColor)
	return os.WriteFile(path, []byte(content), 0600)
}

func validUserColor(value string) bool {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "#") {
		if len(value) != 7 {
			return false
		}
		_, err := strconv.ParseUint(value[1:], 16, 24)
		return err == nil
	}
	n, err := strconv.Atoi(value)
	return err == nil && n >= 0 && n <= 255
}

func terminalBellCmd() tea.Cmd {
	return func() tea.Msg {
		_, _ = os.Stdout.Write([]byte{'\a'})
		return nil
	}
}

// -----------------------------------------------------------------------------
// Wire protocol
// -----------------------------------------------------------------------------

type User struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type Packet struct {
	Type       string    `json:"type"`
	Room       string    `json:"room,omitempty"`
	User       User      `json:"user,omitempty"`
	Text       string    `json:"text,omitempty"`
	Time       time.Time `json:"time,omitempty"`
	Typing     bool      `json:"typing,omitempty"`
	AddHistory bool      `json:"add_history,omitempty"`
	Ephemeral  bool      `json:"ephemeral,omitempty"`
}

// -----------------------------------------------------------------------------
// Server
// -----------------------------------------------------------------------------

type serverClient struct {
	enc       *json.Encoder
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
	mu         sync.Mutex
	rooms      map[string]*roomState
	addHistory bool
}

func newServer(addHistory bool) *server {
	return &server{
		rooms:      make(map[string]*roomState),
		addHistory: addHistory,
	}
}

func (s *server) room(name string) *roomState {
	r, ok := s.rooms[name]

	if !ok {
		r = &roomState{
			clients: make(map[string]*serverClient),
		}

		s.rooms[name] = r
	}

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

func (c *serverClient) send(packet Packet) {
	c.mu.Lock()
	defer c.mu.Unlock()

	_ = c.enc.Encode(packet)
}

func (s *server) systemMessage(
	roomName string,
	text string,
) {
	packet := Packet{
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

	encoder := json.NewEncoder(conn)

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
		enc: encoder,
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
	room := s.room(roomName)
	if hello.AddHistory {
		room.historyEnabled = true
	}
	user.Name = uniqueUserName(room, user.Name)
	client.user = user
	room.clients[client.id] = client
	var history []Packet
	if room.historyEnabled {
		history = append([]Packet(nil), room.messages...)
	}

	// Hold this client's write lock before releasing the server lock. Any
	// concurrent broadcast will wait behind the welcome/history sequence,
	// while other rooms remain free to proceed.
	client.mu.Lock()
	s.mu.Unlock()

	writeErr := client.enc.Encode(Packet{
		Type: "welcome",
		Room: roomName,
		User: client.user,
	})
	if writeErr == nil {
		for _, message := range history {
			if writeErr = client.enc.Encode(message); writeErr != nil {
				break
			}
		}
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

	client.mu.Unlock()
	if writeErr != nil {
		s.removeClient(client)
		return
	}

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

	for {
		var packet Packet

		if err := decoder.Decode(&packet); err != nil {
			s.removeClient(client)
			return
		}

		switch packet.Type {

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
				Type: "message",
				Room: roomName,
				User: user,
				Text: text,
				Time: time.Now().UTC(),
			}

			s.mu.Lock()

			room := s.room(roomName)
			if room.historyEnabled {
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

		case "me":
			text := strings.TrimSpace(packet.Text)
			if text == "" {
				continue
			}
			if len([]rune(text)) > maxMessage {
				text = string([]rune(text)[:maxMessage])
			}
			action := Packet{Type: "me", Room: roomName, User: user, Text: text, Time: time.Now().UTC()}
			s.mu.Lock()
			room := s.room(roomName)
			if room.historyEnabled {
				room.messages = append(room.messages, action)
			}
			client.typing = false
			s.mu.Unlock()
			s.broadcast(roomName, action)
			s.broadcast(roomName, Packet{Type: "typing", Room: roomName, User: user, Typing: false})

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

		case "who":
			// Return the current room roster only to the requesting client.
			s.mu.Lock()
			room := s.rooms[roomName]
			var names []string
			if room != nil {
				for _, connected := range room.clients {
					names = append(names, connected.user.Name)
				}
			}
			s.mu.Unlock()

			sort.Slice(names, func(i, j int) bool {
				return strings.ToLower(names[i]) < strings.ToLower(names[j])
			})

			text := fmt.Sprintf("Connected users (%d):", len(names))
			for _, name := range names {
				text += "\n  • " + name
			}
			client.send(Packet{Type: "system", Room: roomName, Text: text, Time: time.Now().UTC()})

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

func runServer(addHistory bool) error {
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

	s := newServer(addHistory)

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

// -----------------------------------------------------------------------------
// Client connection
// -----------------------------------------------------------------------------

type wireClient struct {
	conn    net.Conn
	enc     *json.Encoder
	decoder *json.Decoder
	mu      sync.Mutex
}

func (c *wireClient) send(
	packet Packet,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.enc.Encode(packet)
}

func connectToServer(
	room string,
	user User,
	addHistory bool,
	ephemeral bool,
) (*wireClient, error) {
	conn, err := net.Dial(
		"unix",
		socketPath,
	)

	if err != nil {
		// Start the internal server because nobody is currently running it.
		args := []string{"--daemon"}
		if addHistory {
			args = append(args, "--add-history")
		}
		cmd := exec.Command(os.Args[0], args...)

		devNull, openErr := os.OpenFile(
			os.DevNull,
			os.O_RDWR,
			0,
		)

		if openErr != nil {
			return nil, err
		}

		cmd.Stdin = devNull
		cmd.Stdout = devNull
		cmd.Stderr = devNull

		if startErr := cmd.Start(); startErr != nil {
			_ = devNull.Close()
			return nil, startErr
		}

		_ = devNull.Close()

		var lastErr error

		for i := 0; i < 40; i++ {
			time.Sleep(50 * time.Millisecond)

			conn, lastErr = net.Dial(
				"unix",
				socketPath,
			)

			if lastErr == nil {
				break
			}
		}

		if conn == nil {
			if lastErr != nil {
				return nil, lastErr
			}

			return nil, fmt.Errorf(
				"timed out connecting to Zealot daemon",
			)
		}
	}

	client := &wireClient{
		conn: conn,
		enc:  json.NewEncoder(conn),
		decoder: json.NewDecoder(
			bufio.NewReader(conn),
		),
	}

	if err := client.send(Packet{
		Type:       "hello",
		Room:       room,
		User:       user,
		AddHistory: addHistory,
		Ephemeral:  ephemeral,
	}); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return client, nil
}

// -----------------------------------------------------------------------------
// UI messages
// -----------------------------------------------------------------------------

type messageReceivedMsg Packet

type connectionClosedMsg struct{}

type typingClearMsg struct {
	generation int
}

type notificationsReadyMsg struct{}

type scrollbarHoldMsg struct{ generation int }

// -----------------------------------------------------------------------------
// UI styles
// -----------------------------------------------------------------------------

type uiStyles struct {
	app      lipgloss.Style
	time     lipgloss.Style
	inputBox lipgloss.Style
	status   lipgloss.Style
	border   lipgloss.Style
	accent   lipgloss.Style
	system   lipgloss.Style
}

func makeStyles(cfg Config) uiStyles {
	bg := lipgloss.Color(cfg.Background)
	text := lipgloss.Color(cfg.TextColor)
	border := lipgloss.Color(cfg.BorderColor)
	accent := lipgloss.Color(cfg.AccentColor)

	return uiStyles{
		app: lipgloss.NewStyle().
			Background(bg).
			Foreground(text),

		time: lipgloss.NewStyle().
			Background(bg).
			Foreground(lipgloss.Color("#707786")),

		inputBox: lipgloss.NewStyle().
			Background(bg).
			Foreground(text).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			PaddingLeft(1).
			PaddingRight(1),

		status: lipgloss.NewStyle().
			Background(bg).
			Foreground(lipgloss.Color("#707786")).
			PaddingLeft(1).
			PaddingRight(1),

		border: lipgloss.NewStyle().
			Background(bg).
			Foreground(text).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border),

		accent: lipgloss.NewStyle().
			Background(bg).
			Foreground(accent).
			Bold(true),

		system: lipgloss.NewStyle().
			Background(bg).
			Foreground(lipgloss.Color("#707786")).
			Italic(true),
	}
}

// -----------------------------------------------------------------------------
// UI model
// -----------------------------------------------------------------------------

type model struct {
	cfg          Config
	room         string
	roomExplicit bool
	user         User
	onlineCount  int

	client *wireClient

	messages []Packet

	// Rendered rows are cached for the current terminal width. New messages
	// append only their own rows; a width change rebuilds the cache once.
	renderedLines    []string
	renderCacheWidth int
	renderCacheValid bool

	typing      map[string]User
	typingOrder []string

	input textarea.Model

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
		m.resizeInput()
		if oldWidth != m.width || !m.renderCacheValid {
			m.rebuildMessageCache()
			m.clampScroll()
		}

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

		case "message", "system", "me", "letter":
			m.appendMessage(packet)

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

		if packet.Type == "message" &&
			packet.User.Name != m.user.Name &&
			m.notificationsReady &&
			(m.cfg.Bell) {
			return m, tea.Batch(next, terminalBellCmd())
		}

		return m, next
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
		if mouse.Button == tea.MouseLeft && m.scrollbarContains(mouse.X, mouse.Y) && m.maxScroll() > 0 {
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

	case tea.MouseMotionMsg:
		if m.scrollDragging && m.scrollbarContains(mouse.X, mouse.Y) {
			_, thumbHeight, trackHeight := m.scrollbarThumb()
			target := mouse.Y - 3 - m.scrollDragOffset
			if target < 0 { target = 0 }
			if target > trackHeight-thumbHeight { target = trackHeight-thumbHeight }
			m.setScrollFromThumbTop(target)
			return m, nil
		}

	case tea.MouseReleaseMsg:
		m.scrollDragging = false
		m.scrollbarHoldDirection = 0
		m.scrollbarHoldGeneration++
		return m, nil

	case scrollbarHoldMsg:
		if mouse.generation == m.scrollbarHoldGeneration && m.scrollbarHoldDirection != 0 {
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

		case "ctrl+c", "ctrl+q":
			m.sendTyping(false)
			_ = m.client.conn.Close()

			return m, tea.Quit

		case "pgup":
			m.scroll += m.messageHeight() / 2
			m.clampScroll()

			return m, nil

		case "pgdown":
			m.scroll -= m.messageHeight() / 2
			m.clampScroll()

			return m, nil

		case "ctrl+home":
			m.scroll = m.maxScroll()
			return m, nil

		case "ctrl+end":
			m.scroll = 0
			return m, nil

		case "shift+enter", "ctrl+j":
			m.input.InsertString("\n")
			handledNewline = true

		case "enter":
			text := strings.TrimSpace(
				m.input.Value(),
			)

			if text != "" && m.connected {
				m.sendMessage(text)

				m.input.Reset()

				// Sending leaves the input empty, so return
				// the cursor to its static state.
				m.stopCursorBlink()

				if m.isTyping {
					m.isTyping = false
					m.sendTyping(false)
				}
			}

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

func (m *model) sendMessage(text string) {
	trimmed := strings.TrimSpace(text)
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return
	}
	switch strings.ToLower(fields[0]) {
	case "/who":
		if len(fields) != 1 {
			m.appendMessage(Packet{Type: "system", Text: "Usage: /who", Time: time.Now()})
			return
		}
		_ = m.client.send(Packet{Type: "who", Room: m.room})
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
	_ = m.client.send(Packet{Type: "message", Room: m.room, User: m.user, Text: text})
}

// -----------------------------------------------------------------------------
// UI layout
// -----------------------------------------------------------------------------

func (m *model) resizeInput() {
	if m.width < 20 {
		return
	}

	width := m.width - 6 // Box border + padding + the two-column first-row prompt.

	if width < 10 {
		width = 10
	}

	m.input.SetWidth(width)
}

func (m model) messageHeight() int {
	// Header:       1 line
	// Chat:         variable (includes the permanent typing line)
	// Input box: configured content rows plus its top/bottom borders
	// Status:       1 line
	//
	// Lip Gloss Height() sizes the content area; borders add their own rows.
	inputHeight := m.cfg.InputHeight
	if inputHeight < 1 {
		inputHeight = 1
	}
	if inputHeight > 8 {
		inputHeight = 8
	}
	h := m.height - inputHeight - 6

	if h < 3 {
		return 3
	}

	return h
}

func (m *model) appendMessage(packet Packet) {
	m.messages = append(m.messages, packet)

	if m.width < 1 {
		m.renderCacheValid = false
		return
	}

	newLines := m.renderPacketLines(packet, m.width)
	if m.scroll > 0 {
		// Scroll is an offset from the bottom. Increase it by the number of
		// appended rows so the same historical rows remain on screen while
		// the user is reading back. At zero, continue following the live tail.
		m.scroll += len(newLines)
	}

	if m.renderCacheValid && m.renderCacheWidth == m.width {
		m.renderedLines = append(m.renderedLines, newLines...)
	} else {
		m.renderCacheValid = false
	}
	m.clampScroll()
}

func (m *model) rebuildMessageCache() {
	if m.width < 1 {
		m.renderedLines = nil
		m.renderCacheValid = false
		return
	}

	lines := make([]string, 0, len(m.messages))
	for _, message := range m.messages {
		lines = append(lines, m.renderPacketLines(message, m.width)...)
	}
	m.renderedLines = lines
	m.renderCacheWidth = m.width
	m.renderCacheValid = true
}

func (m model) maxScroll() int {
	lines := m.renderMessageLines()

	// One line is permanently reserved for typing.
	visibleHeight := m.messageHeight() - 1

	if visibleHeight < 1 {
		visibleHeight = 1
	}

	max := len(lines) - visibleHeight

	if max < 0 {
		return 0
	}

	return max
}

func (m *model) clampScroll() {
	max := m.maxScroll()
	if m.scroll < 0 { m.scroll = 0 }
	if m.scroll > max { m.scroll = max }
}

func (m model) scrollbarContains(x, y int) bool {
	return x == m.width-2 && y >= 2 && y <= 1+m.messageHeight()
}

func (m model) scrollbarThumb() (top, thumbHeight, trackHeight int) {
	trackHeight = m.messageHeight() - 2
	if trackHeight < 1 { trackHeight = 1 }
	total := len(m.renderMessageLines())
	visible := m.messageHeight() - 1
	if visible < 1 { visible = 1 }
	if total <= visible { return 0, trackHeight, trackHeight }

	thumbHeight = visible * trackHeight / total
	if thumbHeight < 1 { thumbHeight = 1 }
	if thumbHeight > trackHeight { thumbHeight = trackHeight }
	maxTop := trackHeight - thumbHeight
	maxScroll := m.maxScroll()
	if maxScroll > 0 {
		top = (maxScroll-m.scroll) * maxTop / maxScroll
	}
	return
}

func (m *model) setScrollFromTrack(trackPosition int) {
	_, thumbHeight, trackHeight := m.scrollbarThumb()
	m.setScrollFromThumbTop(trackPosition-thumbHeight/2)
	_ = trackHeight
}

func (m *model) setScrollFromThumbTop(top int) {
	_, thumbHeight, trackHeight := m.scrollbarThumb()
	maxTop := trackHeight-thumbHeight
	if top < 0 { top = 0 }
	if top > maxTop { top = maxTop }
	if maxTop > 0 {
		m.scroll = m.maxScroll() - top*m.maxScroll()/maxTop
	} else {
		m.scroll = 0
	}
	m.clampScroll()
}

func (m model) scrollbarHoldCmd(generation int) tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return scrollbarHoldMsg{generation: generation}
	})
}

// -----------------------------------------------------------------------------
// Message rendering
// -----------------------------------------------------------------------------

func (m model) renderMessageLines() []string {
	if m.renderCacheValid && m.renderCacheWidth == m.width {
		return m.renderedLines
	}

	return m.renderAllMessageLines()
}

func (m model) renderAllMessageLines() []string {
	var lines []string
	for _, message := range m.messages {
		lines = append(lines, m.renderPacketLines(message, m.width)...)
	}
	return lines
}

// renderChatRow keeps every rendered chat row inside the chat viewport.
// Width() alone does not clip overlong content, so wrap functions should keep
// rows bounded and this final guard prevents a malformed/oversized row from
// widening the joined TUI layout.
func renderChatRow(rowFill lipgloss.Style, line string, width int) string {
	if width < 1 {
		width = 1
	}
	return rowFill.Render(truncateANSI(line, width))
}

// ansiSequenceEnd returns the byte index immediately after the ANSI escape
// sequence beginning at i. It is deliberately small and dependency-free.
func ansiSequenceEnd(s string, i int) int {
	if i >= len(s) || s[i] != 0x1b {
		return i
	}
	i++
	if i >= len(s) {
		return i
	}
	switch s[i] {
	case '[': // CSI, commonly SGR color/style sequences
		i++
		for i < len(s) {
			b := s[i]
			i++
			if b >= 0x40 && b <= 0x7e {
				break
			}
		}
	case ']': // OSC, terminated by BEL or ST
		i++
		for i < len(s) {
			if s[i] == 0x07 {
				return i + 1
			}
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
			i++
		}
	default:
		i++ // other two-byte escape sequence
	}
	return i
}

// truncateANSI clips a line to its terminal display width without cutting an
// ANSI escape sequence in half.
func truncateANSI(s string, width int) string {
	if width < 1 {
		width = 1
	}
	var out strings.Builder
	used := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			end := ansiSequenceEnd(s, i)
			out.WriteString(s[i:end])
			i = end
			continue
		}

		_, size := utf8.DecodeRuneInString(s[i:])
		if size < 1 {
			break
		}
		piece := s[i : i+size]
		runeWidth := lipgloss.Width(piece)
		if used+runeWidth > width {
			out.WriteString("\x1b[0m")
			break
		}
		out.WriteString(piece)
		used += runeWidth
		i += size
	}
	return out.String()
}

// wrapANSIHardLine wraps terminal output at display-cell boundaries while
// preserving its visible characters and ANSI sequences. It is intended for
// piped letters/terminal output, where preserving literal spaces is more
// important than conventional word wrapping (ASCII art may be distorted).
func wrapANSIHardLine(s string, width int) []string {
	if width < 1 {
		width = 1
	}

	var lines []string
	var current strings.Builder
	used := 0
	activeSGR := ""

	finishLine := func() {
		if activeSGR != "" {
			current.WriteString("\x1b[0m")
		}
		lines = append(lines, current.String())
		current.Reset()
		if activeSGR != "" {
			// Most terminal output (including lolcat) uses a fresh SGR color
			// before its next colored run. Reapply the latest style at a wrap.
			current.WriteString(activeSGR)
		}
		used = 0
	}

	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			end := ansiSequenceEnd(s, i)
			seq := s[i:end]
			current.WriteString(seq)
			if strings.HasPrefix(seq, "\x1b[") && strings.HasSuffix(seq, "m") {
				params := strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b["), "m")
				reset := params == ""
				for _, param := range strings.Split(params, ";") {
					if param == "0" {
						reset = true
						break
					}
				}
				if reset && (params == "" || params == "0") {
					activeSGR = ""
				} else {
					activeSGR = seq
				}
			}
			i = end
			continue
		}

		_, size := utf8.DecodeRuneInString(s[i:])
		if size < 1 {
			break
		}
		piece := s[i : i+size]
		pieceWidth := lipgloss.Width(piece)
		if used > 0 && used+pieceWidth > width {
			finishLine()
		}
		current.WriteString(piece)
		used += pieceWidth
		i += size
	}

	if current.Len() > 0 || len(lines) == 0 {
		if activeSGR != "" {
			current.WriteString("\x1b[0m")
		}
		lines = append(lines, current.String())
	}
	return lines
}

func (m model) renderPacketLines(msg Packet, width int) []string {
	// The chat box has a one-column border on each side. Lip Gloss Width is
	// the final outer width, so the interior rows must be exactly width-2.
	contentWidth := width - 2
	if contentWidth < 1 {
		contentWidth = 1
	}

	bg := lipgloss.Color(m.cfg.Background)
	text := lipgloss.Color(m.cfg.TextColor)
	rowFill := lipgloss.NewStyle().Width(contentWidth).Background(bg)
	bodyStyle := lipgloss.NewStyle().Foreground(text).Background(bg)

	if msg.Type == "letter" {
		header := fmt.Sprintf("  [%s] sent a letter:", msg.User.Name)
		lines := []string{renderChatRow(rowFill, m.styles.system.Render(header), contentWidth)}
		indent := 4
		if contentWidth <= 4 {
			indent = 0
		} else if contentWidth < 14 {
			indent = 2
		}
		bodyWidth := contentWidth - indent
		if bodyWidth < 1 {
			bodyWidth = 1
		}
		for _, line := range strings.Split(msg.Text, "\n") {
			line = strings.TrimSuffix(line, "\r")
			// Tabs expand to spaces so a terminal tab stop cannot push a row
			// outside the viewport. Literal spaces are otherwise preserved.
			line = strings.ReplaceAll(line, "\t", "    ")
			wrapped := wrapANSIHardLine(line, bodyWidth)
			for _, part := range wrapped {
				lines = append(lines, renderChatRow(rowFill, strings.Repeat(" ", indent)+part+"\x1b[0m", contentWidth))
			}
		}
		return lines
	}

	if msg.Type == "system" {
		systemText := fmt.Sprintf(
			"[%s] %s",
			msg.Time.Local().Format("15:04"),
			msg.Text,
		)
		wrapped := wrapText(systemText, contentWidth-2)
		lines := make([]string, 0, len(wrapped))
		for _, line := range wrapped {
			styled := m.styles.system.Render("  " + line)
			lines = append(lines, renderChatRow(rowFill, styled, contentWidth))
		}
		return lines
	}

	if msg.Type == "me" {
		line := fmt.Sprintf("[%s] * %s %s", msg.Time.Local().Format("15:04"), msg.User.Name, msg.Text)
		wrapped := wrapText(line, contentWidth-2)
		lines := make([]string, 0, len(wrapped))
		for _, part := range wrapped {
			lines = append(lines, renderChatRow(rowFill, m.styles.system.Render("  "+part), contentWidth))
		}
		return lines
	}

	timeText := msg.Time.Local().Format("15:04")
	nameStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(msg.User.Color)).
		Background(bg).
		Bold(true)

	// In narrow terminals, lead with the username and move the timestamp to
	// the next row. This preserves room for readable body text instead of
	// allowing the timestamp + username prefix to consume the entire width.
	if width <= 48 {
		return renderCompactMessage(rowFill, bodyStyle, nameStyle, m.styles.time, msg, timeText, contentWidth)
	}

	timestamp := m.styles.time.Render("[" + timeText + "]")
	separator := bodyStyle.Render(" ")
	name := nameStyle.Render(msg.User.Name)
	suffix := bodyStyle.Render(": ")
	prefix := timestamp + separator + name + suffix
	prefixWidth := lipgloss.Width(prefix)
	if prefixWidth >= contentWidth {
		return renderCompactMessage(rowFill, bodyStyle, nameStyle, m.styles.time, msg, timeText, contentWidth)
	}

	wrapped := wrapText(msg.Text, contentWidth-prefixWidth)
	if len(wrapped) == 0 {
		wrapped = []string{""}
	}

	lines := make([]string, 0, len(wrapped))
	lines = append(lines, renderChatRow(rowFill, prefix+bodyStyle.Render(wrapped[0]), contentWidth))
	indent := strings.Repeat(" ", prefixWidth)
	for _, line := range wrapped[1:] {
		lines = append(lines, renderChatRow(rowFill, bodyStyle.Render(indent+line), contentWidth))
	}
	return lines
}

// renderCompactMessage uses a two-row message header on terminals 48 columns
// wide or narrower: "Name: first part" followed by "[HH:MM] continuation".
// Further continuation rows start at the left edge to reclaim horizontal room.
func renderCompactMessage(
	rowFill lipgloss.Style,
	bodyStyle lipgloss.Style,
	nameStyle lipgloss.Style,
	timeStyle lipgloss.Style,
	msg Packet,
	timeText string,
	contentWidth int,
) []string {
	if contentWidth < 1 {
		contentWidth = 1
	}
	nameWidth := contentWidth - 2 // reserve space for the colon and optional body gap
	if nameWidth < 1 {
		nameWidth = 1
	}
	nameParts := wrapText(msg.User.Name, nameWidth)
	if len(nameParts) == 0 {
		nameParts = []string{""}
	}
	stamp := "[" + timeText + "]"
	lines := make([]string, 0, 4)

	if len(nameParts) > 1 {
		// Preserve very long usernames by hyphen-wrapping them rather than
		// silently chopping the identity off at the viewport edge.
		for _, part := range nameParts[:len(nameParts)-1] {
			lines = append(lines, renderChatRow(rowFill, nameStyle.Render(part), contentWidth))
		}
		lines = append(lines, renderChatRow(rowFill, nameStyle.Render(nameParts[len(nameParts)-1]+":"), contentWidth))
		wrapped := wrapTextVariable(msg.Text, 0, contentWidth-lipgloss.Width(stamp)-1, contentWidth)
		if len(wrapped) < 2 {
			lines = append(lines, renderChatRow(rowFill, timeStyle.Render(stamp), contentWidth))
			return lines
		}
		continuation := timeStyle.Render(stamp)
		if wrapped[1] != "" {
			continuation += bodyStyle.Render(" " + wrapped[1])
		}
		lines = append(lines, renderChatRow(rowFill, continuation, contentWidth))
		for _, part := range wrapped[2:] {
			lines = append(lines, renderChatRow(rowFill, bodyStyle.Render(part), contentWidth))
		}
		return lines
	}

	label := nameParts[0] + ": "
	firstWidth := contentWidth - lipgloss.Width(label)
	wrapped := wrapTextVariable(msg.Text, firstWidth, contentWidth-lipgloss.Width(stamp)-1, contentWidth)
	if len(wrapped) == 0 {
		wrapped = []string{""}
	}
	firstRow := nameStyle.Render(nameParts[0] + ":")
	if wrapped[0] != "" {
		firstRow += bodyStyle.Render(" " + wrapped[0])
	}
	lines = append(lines, renderChatRow(rowFill, firstRow, contentWidth))

	if len(wrapped) < 2 {
		lines = append(lines, renderChatRow(rowFill, timeStyle.Render(stamp), contentWidth))
		return lines
	}
	continuation := timeStyle.Render(stamp)
	if wrapped[1] != "" {
		continuation += bodyStyle.Render(" " + wrapped[1])
	}
	lines = append(lines, renderChatRow(rowFill, continuation, contentWidth))
	for _, part := range wrapped[2:] {
		lines = append(lines, renderChatRow(rowFill, bodyStyle.Render(part), contentWidth))
	}
	return lines
}

func wrapText(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	return wrapTextVariable(text, width, width, width)
}

// wrapTextVariable wraps prose into rows with a possibly different capacity for
// the first and second row. It prefers spaces between words; only tokens that
// cannot fit on an otherwise empty row are split. URL-like tokens prefer
// punctuation breakpoints and are never given synthetic hyphens.
func wrapTextVariable(text string, firstWidth, secondWidth, fullWidth int) []string {
	if firstWidth < 0 {
		firstWidth = 0
	}
	if secondWidth < 1 {
		secondWidth = 1
	}
	if fullWidth < 1 {
		fullWidth = 1
	}
	lineWidth := func(lineIndex int) int {
		switch lineIndex {
		case 0:
			return firstWidth
		case 1:
			return secondWidth
		default:
			return fullWidth
		}
	}

	var result []string
	current := ""
	for _, paragraph := range strings.Split(text, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			if current != "" {
				result = append(result, current)
				current = ""
			}
			result = append(result, "")
			continue
		}

		for _, token := range words {
			remaining := token
			for remaining != "" {
				capWidth := lineWidth(len(result))
				if capWidth < 1 {
					// A zero-width first row means the message must begin on the
					// next row (for example, when a long username fills row one).
					if current != "" {
						result = append(result, current)
						current = ""
						continue
					}
					result = append(result, "")
					continue
				}

				if current != "" {
					candidate := current + " " + remaining
					if lipgloss.Width(candidate) <= capWidth {
						current = candidate
						remaining = ""
						break
					}

					result = append(result, current)
					current = ""
					continue
				}

				if lipgloss.Width(remaining) <= capWidth {
					current = remaining
					remaining = ""
					break
				}

				part, rest := splitLongWordPrefix(remaining, capWidth, isURLLike(token))
				if part == "" || rest == remaining {
					// Progress even for unusual zero-width/control-only tokens.
					_, size := utf8.DecodeRuneInString(remaining)
					if size < 1 {
						remaining = ""
						break
					}
					part, rest = remaining[:size], remaining[size:]
				}
				result = append(result, part)
				remaining = rest
			}
		}
		if current != "" {
			result = append(result, current)
			current = ""
		}
	}
	if current != "" {
		result = append(result, current)
	}
	if len(result) == 0 {
		return []string{""}
	}
	return result
}

func splitLongWordPrefix(word string, width int, urlLike bool) (string, string) {
	if width < 1 || word == "" {
		return "", word
	}
	used := 0
	lastFit := 0
	lastPunctuation := 0

	for i := 0; i < len(word); {
		if word[i] == 0x1b {
			i = ansiSequenceEnd(word, i)
			lastFit = i
			continue
		}
		r, size := utf8.DecodeRuneInString(word[i:])
		if size < 1 {
			break
		}
		pieceWidth := lipgloss.Width(string(r))
		if used+pieceWidth > width {
			break
		}
		used += pieceWidth
		i += size
		lastFit = i
		if strings.ContainsRune("./\\-_?:&=#%+@,;~!$'()[]{}", r) {
			lastPunctuation = i
		}
	}

	if lastPunctuation > 0 && lastPunctuation < len(word) {
		return word[:lastPunctuation], word[lastPunctuation:]
	}

	// For URLs/domains, don't insert synthetic hyphens: that would alter the
	// visible address when someone copies it from the terminal.
	cut := lastFit
	if !urlLike && width > 1 {
		cut = 0
		used = 0
		for i := 0; i < len(word); {
			if word[i] == 0x1b {
				i = ansiSequenceEnd(word, i)
				cut = i
				continue
			}
			r, size := utf8.DecodeRuneInString(word[i:])
			if size < 1 {
				break
			}
			pieceWidth := lipgloss.Width(string(r))
			if used+pieceWidth > width-1 {
				break
			}
			used += pieceWidth
			i += size
			cut = i
		}
	}
	if cut <= 0 {
		_, size := utf8.DecodeRuneInString(word)
		if size < 1 {
			return "", word
		}
		cut = size
	}
	part := word[:cut]
	rest := word[cut:]
	if !urlLike && width > 1 && part != "" {
		part += "-"
	}
	return part, rest
}

func isURLLike(word string) bool {
	lower := strings.ToLower(word)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "ftp://") ||
		strings.HasPrefix(lower, "www.") ||
		strings.Contains(word, ".") ||
		strings.Contains(word, "/") ||
		strings.ContainsAny(word, "?&=#")
}

// -----------------------------------------------------------------------------
// Typing indicator
// -----------------------------------------------------------------------------

func (m model) typingText() string {
	if len(m.typingOrder) == 0 {
		return ""
	}

	names := make([]string, 0, len(m.typingOrder))
	for _, name := range m.typingOrder {
		if _, ok := m.typing[name]; ok {
			names = append(names, name)
		}
	}

	if len(names) == 0 {
		return ""
	}
	if len(names) == 1 {
		return names[0] + " is typing…"
	}
	if len(names) == 2 {
		return names[0] + " and " + names[1] + " are typing…"
	}
	return fmt.Sprintf("%d people are typing…", len(names))
}

// -----------------------------------------------------------------------------
// Header
// -----------------------------------------------------------------------------

func (m model) renderHeader() string {
	if m.width < 1 {
		return ""
	}

	var title string
	if m.roomExplicit {
		title = "ZEALOT — " + m.room
	} else {
		title = "ZEALOT"
	}

	online := fmt.Sprintf("Online: %d", m.onlineCount)
	padding := m.width - lipgloss.Width(title) - lipgloss.Width(online)

	if padding < 1 {
		padding = 1
	}

	line := title + strings.Repeat(" ", padding) + online

	return m.styles.accent.
		Width(m.width).
		Render(line)
}

// -----------------------------------------------------------------------------
// Main UI
// -----------------------------------------------------------------------------

func appView(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return appView("Starting Zealot…")
	}

	// -------------------------------------------------------------------------
	// Header
	// -------------------------------------------------------------------------

	header := m.renderHeader()

	// -------------------------------------------------------------------------
	// Chat
	// -------------------------------------------------------------------------

	lines := m.renderMessageLines()

	// ALWAYS reserve one line for the typing indicator.
	//
	// When nobody is typing, this line is blank.
	// When somebody starts typing, the indicator appears on exactly that
	// same line.
	chatHeight := m.messageHeight() - 1

	if chatHeight < 1 {
		chatHeight = 1
	}

	start := len(lines) -
		chatHeight -
		m.scroll

	if start < 0 {
		start = 0
	}

	end := start + chatHeight

	if end > len(lines) {
		end = len(lines)
	}

	// Copy the visible window before padding it; lines may be the cached slice,
	// and appending to a view into that slice could overwrite cached history.
	visible := append([]string(nil), lines[start:end]...)
	chatBlank := lipgloss.NewStyle().
		Width(m.width - 2).
		Background(lipgloss.Color(m.cfg.Background)).
		Render("")

	for len(visible) < chatHeight {
		visible = append(visible, chatBlank)
	}

	// Reserve the final interior column for a permanent scrollbar rail.
	// The thumb only appears once the message history exceeds the viewport.
	_, thumbHeight, trackHeight := m.scrollbarThumb()
	thumbTop, _, _ := m.scrollbarThumb()
	scrollbarStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.cfg.BorderColor)).
		Background(lipgloss.Color(m.cfg.Background))
	scrollbarRows := make([]string, chatHeight+1)
	for i := range scrollbarRows {
		switch {
		case i == 0:
			scrollbarRows[i] = scrollbarStyle.Render("^")
		case i == chatHeight:
			scrollbarRows[i] = scrollbarStyle.Render("v")
		default:
			trackPosition := i-1
			glyph := "│"
			if m.maxScroll() > 0 && trackPosition >= thumbTop && trackPosition < thumbTop+thumbHeight {
				glyph = "█"
			}
			if m.maxScroll() == 0 || trackPosition >= trackHeight {
				glyph = " "
			}
			scrollbarRows[i] = scrollbarStyle.Render(glyph)
		}
	}
	for i := range visible {
		visible[i] = truncateANSI(visible[i], m.width-3) + scrollbarRows[i]
	}
	chat := strings.Join(visible, "\n")

	// -------------------------------------------------------------------------
	// Permanent typing line
	// -------------------------------------------------------------------------

	typing := m.typingText()
	typingLine := chatBlank
	if typing != "" {
		typingLine = m.styles.accent.Width(m.width - 3).Render("  " + typing)
	}
	typingLine = truncateANSI(typingLine, m.width-3) + scrollbarRows[chatHeight]
	chat += "\n" + typingLine

	chatBox := m.styles.border.
		Width(m.width).
		Height(m.messageHeight()).
		Render(chat)

	// -------------------------------------------------------------------------
	// Input
	// -------------------------------------------------------------------------

	// Input content rows are configurable; borders add two more rows.
	inputView := strings.Split(strings.TrimSuffix(m.input.View(), "\n"), "\n")
	if len(inputView) == 0 {
		inputView = []string{""}
	}
	prompt := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.cfg.AccentColor)).
		Background(lipgloss.Color(m.cfg.Background)).
		Bold(true).
		Render("> ")
	inputView[0] = prompt + inputView[0]

	// The textarea emits shorter/unstyled end-of-buffer rows when empty. Fill
	// every row's remaining columns with the configured background so those
	// rows match the rest of the composer before the user starts typing.
	inputInnerWidth := m.width - 4 // border (2) + composer padding (2)
	if inputInnerWidth < 1 {
		inputInnerWidth = 1
	}
	inputBackground := lipgloss.NewStyle().Background(lipgloss.Color(m.cfg.Background))
	if strings.TrimSpace(m.input.Value()) == "" {
		// Bubbles' empty-placeholder view uses EndOfBuffer cells for rows below
		// the placeholder. Those cells can retain a terminal-default background
		// through the viewport, even when EndOfBuffer is styled. Replace the
		// lower empty rows wholesale with explicitly background-painted spaces.
		for i := 1; i < len(inputView); i++ {
			inputView[i] = inputBackground.Render(strings.Repeat(" ", inputInnerWidth))
		}
	}
	for i, line := range inputView {
		remaining := inputInnerWidth - lipgloss.Width(line)
		if remaining > 0 {
			inputView[i] = line + inputBackground.Render(strings.Repeat(" ", remaining))
		}
	}
	// The textarea is configured for exactly InputHeight rows; normalize the
	// slice defensively so a trailing newline never creates an extra row.
	if len(inputView) > m.cfg.InputHeight {
		inputView = inputView[:m.cfg.InputHeight]
	}
	for len(inputView) < m.cfg.InputHeight {
		inputView = append(inputView, inputBackground.Render(strings.Repeat(" ", inputInnerWidth)))
	}
	inputContent := strings.Join(inputView, "\n")
	inputBox := m.styles.inputBox.
		Width(m.width).
		Height(m.cfg.InputHeight).
		Render(inputContent)

	// -------------------------------------------------------------------------
	// Status
	// -------------------------------------------------------------------------

	status := m.styles.status.
		Width(m.width).
		Render(
			m.status +
				"   " +
				"Enter: send  •  PgUp/PgDn: scroll  •  Ctrl+C: quit",
		)

	// -------------------------------------------------------------------------
	// Full application
	// -------------------------------------------------------------------------

	content := m.styles.app.
		Width(m.width).
		Height(m.height).
		Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				header,
				chatBox,
				inputBox,
				status,
			),
		)
	return appView(content)
}

// -----------------------------------------------------------------------------
// Program entry
// -----------------------------------------------------------------------------

func main() {
	daemon := flag.Bool(
		"daemon",
		false,
		"run the internal Zealot daemon",
	)
	addHistory := flag.Bool(
		"add-history",
		false,
		"keep and replay in-memory room chat history (daemon only)",
	)

	roomFlag := flag.String(
		"room",
		"",
		"room to join",
	)

	flag.Parse()

	if *daemon {
		if err := runServer(*addHistory); err != nil {
			os.Exit(1)
		}

		return
	}

	room := cleanRoom(*roomFlag)

	if room == "" {
		room = "lobby"
	}

	cfg, err := loadConfig()

	if err != nil {
		fmt.Fprintln(
			os.Stderr,
			"could not load config:",
			err,
		)

		os.Exit(1)
	}

	user := User{
		Name:  cfg.Name,
		Color: cfg.UserColor,
	}

	stdinInfo, statErr := os.Stdin.Stat()
	if statErr == nil && stdinInfo.Mode()&os.ModeCharDevice == 0 {
		// A pipe or redirected file means this is a one-shot letter send rather
		// than an interactive TUI session.
		data, readErr := os.ReadFile("/dev/stdin")
		if readErr != nil {
			fmt.Fprintln(os.Stderr, "could not read letter from stdin:", readErr)
			os.Exit(1)
		}
		if strings.TrimSpace(string(data)) == "" {
			return
		}
		client, connectErr := connectToServer(room, user, *addHistory, true)
		if connectErr != nil {
			fmt.Fprintln(os.Stderr, "could not connect to Zealot:", connectErr)
			os.Exit(1)
		}
		sendErr := client.send(Packet{Type: "letter", Room: room, Text: string(data)})
		if sendErr == nil {
			// Wait for the daemon to confirm it handled the letter. This also
			// detects a daemon process that was started from an older binary.
			_ = client.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			acked := false
			for !acked {
				var response Packet
				if decodeErr := client.decoder.Decode(&response); decodeErr != nil {
					sendErr = fmt.Errorf("no delivery confirmation from the Zealot daemon; it may still be running an older version. Restart the daemon and try again")
					break
				}
				if response.Type == "letter_ack" {
					acked = true
				}
			}
		}
		_ = client.conn.Close()
		if sendErr != nil {
			fmt.Fprintln(os.Stderr, "could not send letter:", sendErr)
			os.Exit(1)
		}
		return
	}

	client, err := connectToServer(
		room,
		user,
		*addHistory,
		false,
	)

	if err != nil {
		fmt.Fprintln(
			os.Stderr,
			"could not connect to Zealot:",
			err,
		)

		os.Exit(1)
	}

	defer client.conn.Close()

	m := newModel(
		cfg,
		room,
		*roomFlag != "",
		client,
	)

	program := tea.NewProgram(m)

	if _, err := program.Run(); err != nil {
		fmt.Fprintln(
			os.Stderr,
			"Zealot:",
			err,
		)

		os.Exit(1)
	}
}