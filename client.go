package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"
)

// -----------------------------------------------------------------------------
// Client connection
// -----------------------------------------------------------------------------

type wireClient struct {
	conn          net.Conn
	enc           *json.Encoder
	decoder       *json.Decoder
	initialPacket Packet
	hasInitial    bool
	historyReplay bool
	mu            sync.Mutex
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
		// Start an internal server process. The first client handshake chooses
		// the new room's history policy.
		cmd := exec.Command(os.Args[0])
		cmd.Env = append(os.Environ(), "ZEALOT_INTERNAL_DAEMON=1")

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
		Type:      "hello",
		Room:      room,
		User:       user,
		AddHistory: addHistory,
		Ephemeral:  ephemeral,
	}); err != nil {
		_ = conn.Close()
		return nil, err
	}

	// Validate the room policy before returning to the caller, so a mismatch
	// is reported in the terminal without starting the TUI.
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	var response Packet
	if err := client.decoder.Decode(&response); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("could not complete the Zealot handshake: %w", err)
	}
	_ = conn.SetReadDeadline(time.Time{})
	if response.Type == "history_mismatch" {
		_ = conn.Close()
		return nil, errors.New(response.Text)
	}
	if response.Type != "welcome" {
		_ = conn.Close()
		return nil, fmt.Errorf("unexpected response from Zealot: %s", response.Type)
	}
	client.initialPacket = response
	client.hasInitial = true
	client.historyReplay = response.AddHistory

	return client, nil
}
