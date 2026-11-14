package main

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
)

// The daemon, not the sending client, rolls these results. Restrict ranges
// to a practical size and reject malformed requests before drawing anything.
const maxRandomCommandValue int64 = 1_000_000_000

func parseRandomRange(input string) (int64, int64, error) {
	parts := strings.Split(strings.TrimSpace(input), "-")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, 0, fmt.Errorf("Usage: /random MIN-MAX (0 to %d)", maxRandomCommandValue)
	}
	for _, part := range parts {
		for _, char := range part {
			if char < '0' || char > '9' {
				return 0, 0, fmt.Errorf("Usage: /random MIN-MAX (0 to %d)", maxRandomCommandValue)
			}
		}
	}
	minimum, minErr := strconv.ParseInt(parts[0], 10, 64)
	maximum, maxErr := strconv.ParseInt(parts[1], 10, 64)
	if minErr != nil || maxErr != nil || minimum > maximum || maximum > maxRandomCommandValue {
		return 0, 0, fmt.Errorf("Usage: /random MIN-MAX (0 to %d; MIN <= MAX)", maxRandomCommandValue)
	}
	return minimum, maximum, nil
}

func secureRandomBetween(minimum, maximum int64) (int64, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(maximum-minimum+1))
	if err != nil {
		return 0, err
	}
	return minimum + n.Int64(), nil
}

func newCoinflipResult(room string, user User) (Packet, error) {
	value, err := secureRandomBetween(0, 1)
	if err != nil {
		return Packet{}, err
	}
	result := "Heads"
	if value == 1 {
		result = "Tails"
	}
	return Packet{
		ID: nextMessageID(), Type: "coinflip_result", Room: room,
		User: user, Text: result, Time: time.Now().UTC(),
	}, nil
}

func newRandomResult(room string, user User, bounds string) (Packet, error) {
	minimum, maximum, err := parseRandomRange(bounds)
	if err != nil {
		return Packet{}, err
	}
	value, err := secureRandomBetween(minimum, maximum)
	if err != nil {
		return Packet{}, err
	}
	return Packet{
		ID: nextMessageID(), Type: "random_result", Room: room,
		User: user, Text: fmt.Sprintf("%d–%d: %d", minimum, maximum, value),
		Time: time.Now().UTC(),
	}, nil
}

// Only the daemon calls this. Client-supplied *_result packet types are not
// handled by the protocol switch, so clients cannot pick their own results.
func (s *server) broadcastGameResult(roomName string, client *serverClient, result Packet) {
	s.mu.Lock()
	if room := s.rooms[roomName]; room != nil && room.historyEnabled {
		room.messages = append(room.messages, result)
	}
	client.typing = false
	s.mu.Unlock()
	s.broadcast(roomName, result)
	s.broadcast(roomName, Packet{Type: "typing", Room: roomName, User: result.User, Typing: false})
	s.broadcastRoster(roomName)
}
