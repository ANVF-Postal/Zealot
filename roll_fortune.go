package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Dice limits keep server-generated result packets small and avoid large draws.
const (
	maxRollDice  = 20
	maxRollSides = 1000
)

func parseDiceNotation(input string) (count, sides int, err error) {
	usage := fmt.Errorf("Usage: /roll NdM (1–%d dice, 2–%d sides)", maxRollDice, maxRollSides)
	parts := strings.Split(strings.ToLower(strings.TrimSpace(input)), "d")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, 0, usage
	}
	for _, part := range parts {
		for _, r := range part {
			if r < '0' || r > '9' {
				return 0, 0, usage
			}
		}
	}
	count, countErr := strconv.Atoi(parts[0])
	sides, sidesErr := strconv.Atoi(parts[1])
	if countErr != nil || sidesErr != nil || count < 1 || count > maxRollDice || sides < 2 || sides > maxRollSides {
		return 0, 0, usage
	}
	return count, sides, nil
}

func newRollResult(room string, user User, notation string) (Packet, error) {
	count, sides, err := parseDiceNotation(notation)
	if err != nil {
		return Packet{}, err
	}
	values := make([]string, count)
	total := 0
	for i := range values {
		result, drawErr := secureRandomBetween(1, int64(sides))
		if drawErr != nil {
			return Packet{}, fmt.Errorf("Dice roll failed: could not obtain secure randomness")
		}
		values[i] = strconv.FormatInt(result, 10)
		total += int(result)
	}
	return Packet{
		ID: nextMessageID(), Type: "roll_result", Room: room, User: user,
		Text: fmt.Sprintf("%dd%d: %s = %d", count, sides, strings.Join(values, " + "), total),
		Time: time.Now().UTC(),
	}, nil
}

// The daemon selects from a deliberately fixed collection of original,
// slightly absurd fortunes. Clients can only request a draw, never supply it.
var fortuneSayings = []string{
	"You will be blessed by many winds tomorrow.",
	"Your next great achievement will involve a suspicious amount of cheese.",
	"A forgotten terminal will remember your name.",
	"Beware of a duck bearing excellent advice.",
	"An unexpected ally will arrive with snacks.",
	"The next door you open may already be open.",
	"Your keyboard has been plotting a promotion.",
	"A tiny victory will demand an enormous celebration.",
	"Someone nearby will misplace a perfectly good metaphor.",
	"You will find meaning in an improbable sequence of dots.",
	"The universe recommends a brief detour for noodles.",
	"Today, an ordinary pebble will seem unusually wise.",
	"Great fortune approaches, wearing mismatched socks.",
	"Your finest idea will arrive just after you stop looking.",
	"An old joke will become surprisingly relevant.",
	"You will make a powerful friend among the pigeons.",
	"Tomorrow's clouds have already heard about you.",
	"A mysterious stranger will say something obvious.",
	"Trust the cat, but verify its credentials.",
	"The stars suggest you save your work.",
	"One very stubborn problem will surrender to a cup of tea.",
	"Your next adventure begins with a misplaced comma.",
	"The wind carries news, but refuses to cite its sources.",
	"A ridiculous plan will be remembered fondly.",
}

func newFortuneResult(room string, user User, target string) (Packet, error) {
	draw, err := secureRandomBetween(0, int64(len(fortuneSayings)-1))
	if err != nil {
		return Packet{}, fmt.Errorf("Fortune failed: could not obtain secure randomness")
	}
	packet := Packet{
		ID: nextMessageID(), Type: "fortune_result", Room: room, User: user,
		Text: fortuneSayings[draw], FortuneTo: target, Time: time.Now().UTC(),
	}
	if target != "" {
		packet.Mentions = []string{target}
	}
	return packet, nil
}

// The target must be a current room member. Resolve against the daemon's
// canonical names, never a claimed identity supplied in the request packet.
func (s *server) resolveFortuneTarget(roomName, requested string) (string, bool) {
	if requested == "" {
		return "", true
	}
	if !strings.HasPrefix(requested, "@") || len(requested) == 1 {
		return "", false
	}
	name := requested[1:]
	s.mu.Lock()
	defer s.mu.Unlock()
	if room := s.rooms[roomName]; room != nil {
		for _, participant := range room.clients {
			if strings.EqualFold(participant.user.Name, name) {
				return participant.user.Name, true
			}
		}
	}
	return "", false
}
