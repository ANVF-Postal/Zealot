package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// appVersion can be set by release builds with -ldflags "-X main.appVersion=1.2.3".
var appVersion = "0.0.0-dev"

// printZealotError uses the same configured accent as the ZEALOT header.
func printZealotError(cfg Config, err error) {
	prefix := lipgloss.NewStyle().
		Foreground(lipgloss.Color(cfg.AccentColor)).
		Render("Zealot:")
	fmt.Fprintln(os.Stderr, prefix, err)
}

// -----------------------------------------------------------------------------
// Program entry
// -----------------------------------------------------------------------------

func main() {
	addHistory := flag.Bool(
		"add-history",
		false,
		"enable in-memory history for a new room (must match an existing room)",
	)

	roomFlag := flag.String(
		"room",
		"",
		"room to join",
	)

	flag.Parse()

	if os.Getenv("ZEALOT_INTERNAL_DAEMON") == "1" {
		if err := runServer(); err != nil {
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
			printZealotError(cfg, connectErr)
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
		printZealotError(cfg, err)
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
