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
	"Bruh",
	"An ancient printer will awaken and demand a better title.",
	"Your luck is buffering, but the progress bar looks promising.",
	"A wise mushroom will approve of your next decision.",
	"The moon has reviewed your plans and requests more snacks.",
	"You will win an argument with a very small goose.",
	"A quiet Tuesday will become suspiciously legendary.",
	"A stranger will compliment your choice of punctuation.",
	"Your future contains a chair that squeaks with authority.",
	"A forgotten password will return in a dream about soup.",
	"The stars have aligned into a mildly concerned face.",
	"You will discover treasure in a drawer full of cables.",
	"Your next typo will accidentally improve the sentence.",
	"A friendly crow will keep your secrets, mostly.",
	"A magnificent opportunity will arrive wearing slippers.",
	"An old computer will finally forgive you.",
	"Someone will offer you a cookie with diplomatic importance.",
	"The universe has placed your luck in the express queue.",
	"Your next cup of coffee will contain a small revelation.",
	"A bold decision will be celebrated by three confused squirrels.",
	"The planets advise you to stop arguing with the toaster.",
	"You will meet a person who laughs at exactly the wrong moment.",
	"A forgotten song will make today a little brighter.",
	"Your socks will soon achieve perfect synchronization.",
	"The wind says your best idea is still in draft.",
	"A modest potato will change your perspective on greatness.",
	"Your patience will be rewarded with an excellent sandwich.",
	"A mystery will be solved by looking under the keyboard.",
	"Your shadow is considering a career in interpretive dance.",
	"A passing cloud will resemble your greatest ambition.",
	"Someone has hidden good luck in your coat pocket.",
	"The next elevator you enter will know more than it admits.",
	"An ordinary spoon will become a symbol of triumph.",
	"Your future self would like you to label your cables.",
	"A heroic snail will inspire an unexpected breakthrough.",
	"Your next mistake will be unusually educational.",
	"The stars predict a dramatic entrance through an ordinary door.",
	"A small bird will mistake you for a person of influence.",
	"Your favorite mug is secretly rooting for you.",
	"An improbable coincidence will involve two umbrellas.",
	"The next rainbow you see will have excellent timing.",
	"A message from the future will contain only a recipe.",
	"Your luck will improve after a short walk and a long snack.",
	"Someone nearby will discover they have been holding a pen upside down.",
	"The universe will reward your curiosity with a very strange fact.",
	"An unexpected compliment will arrive from a person in a hat.",
	"A mysterious humming sound will turn out to be your refrigerator.",
	"Your most stubborn bug will flee at the sight of fresh tea.",
	"The command line will reveal an answer you did not ask for.",
	"You will befriend a ghost who only knows keyboard shortcuts.",
	"A blinking cursor will lead you toward your destiny.",
	"Your terminal will briefly seem wiser than its operator.",
	"The next packet you send will travel with unusual confidence.",
	"An error message will teach you an important life lesson.",
	"Your logs will contain a truth hidden between two warnings.",
	"One day you will achieve enlightenment by closing a tab.",
	"The daemon dreams of electric sheep and better uptime.",
	"A missing semicolon will be found living under the couch.",
	"Your shell history remembers more than your closest friends.",
	"An obscure command will make you feel tremendously powerful.",
	"The next reboot will feel like a fresh beginning.",
	"A rogue process will ask for a vacation.",
	"You will discover that the real treasure was the aliases you made.",
	"Your keyboard will survive a battle with a particularly stubborn crumb.",
	"An unexpected success will be preceded by three failed attempts.",
	"The universe has granted your next command elevated optimism.",
	"Your future includes an unusually satisfying green checkmark.",
	"A tiny bug will teach you an enormous amount of patience.",
	"An old floppy disk will whisper a forgotten prophecy.",
	"Your cursor will blink exactly when you need encouragement.",
	"The correct solution will appear after you stop staring at the screen.",
	"A friendly robot will admire your excellent naming conventions.",
	"A dusty server will sing your praises in hexadecimal.",
	"Today is an auspicious day to back up something important.",
	"An unknown host will turn out to be a very polite neighbor.",
	"Your next deployment will receive a standing ovation from the office plants.",
	"A forgotten comment will explain everything at the perfect moment.",
	"Your fortune is currently undergoing routine maintenance.",
	"A cosmic technician has marked your request as high priority.",
	"Your destiny has been queued behind a dancing penguin.",
	"The stars have requested a larger font size.",
	"A prophecy about you was found scribbled on a napkin.",
	"The universe says yes, but in a surprisingly quiet voice.",
	"A great mystery will be resolved by checking the other pocket.",
	"Your next journey will feature a remarkably helpful bench.",
	"Someone is about to make an excellent pun in your vicinity.",
	"A small inconvenience will disguise a delightful surprise.",
	"The oracle sees a door, a key, and an inexplicable banana.",
	"An invisible committee has voted unanimously in your favor.",
	"Your spirit animal is currently out for lunch.",
	"Beware of advice delivered by a very confident pigeon.",
	"Your next lucky number has requested anonymity.",
	"A peculiar hat will become central to an important conversation.",
	"The moon advises you to take the scenic route.",
	"A nearby houseplant believes in your potential.",
	"A heroic achievement will be commemorated with a tiny sticker.",
	"Your greatest rival will turn out to be an automatic door.",
	"An unexpected breeze will carry the smell of good news.",
	"Your future is bright, but someone forgot to adjust the contrast.",
	"A friendly thunderstorm will applaud your next brilliant thought.",
	"You will find courage in a bowl of surprisingly good noodles.",
	"An unremarkable Tuesday will be talked about for years.",
	"The next person you meet may know a secret about marmalade.",
	"Your confidence will increase by exactly one imaginary unit.",
	"A wizard has misplaced a spell somewhere in your neighborhood.",
	"Your next lucky break will happen during an inconvenient nap.",
	"A great philosopher will agree with you about breakfast.",
	"A cloud shaped like a teapot will offer silent encouragement.",
	"An enthusiastic raccoon will approve of your taste in music.",
	"Your path is clear, except for a suspiciously placed traffic cone.",
	"Your next fortune cookie will contain a shopping list.",
	"An ancient prophecy has been updated for compatibility.",
	"Your future holds seven surprises and one missing sock.",
	"A mysterious envelope will contain absolutely no envelopes.",
	"Your inner compass is pointing toward the nearest bakery.",
	"Your luck will arrive early and pretend it is late.",
	"The universe will soon explain itself through a very long anecdote.",
	"A new friendship will begin with a shared dislike of wet socks.",
	"Your greatest discovery will be hiding in plain sight.",
	"A delightful coincidence will occur between two cups of tea.",
	"The stars have filed a formal compliment about your resilience.",
	"An innocent question will start a magnificent adventure.",
	"A dramatic revelation will involve a surprisingly ordinary stapler.",
	"Your next conversation will include the phrase 'technically speaking'.",
	"A loyal umbrella will save your day without seeking credit.",
	"Someone will tell you a secret that is mostly about ducks.",
	"Your future is being written by a committee of enthusiastic cats.",
	"Good fortune will knock twice, then send a polite text.",
	"A long-forgotten joke will find its perfect audience.",
	"Your next brilliant idea will arrive disguised as a bad one.",
	"The sun has reserved a particularly flattering beam for you.",
	"One small act of kindness will return wearing a funny hat.",
	"A suspiciously perfect pebble will cross your path.",
	"Your destiny includes a nap of historical significance.",
	"The universe encourages you to celebrate the little victories.",
	"An adventurous teabag will introduce you to unexpected wisdom.",
	"You will encounter a staircase that feels unusually encouraging.",
	"Someone will remember your kindness at exactly the right moment.",
	"An old friend will return with a ridiculous story.",
	"Your next dream will feature excellent background music.",
	"An unlikely hero will emerge from the stationery drawer.",
	"The oracle recommends keeping a spare biscuit for emergencies.",
	"A curious fox will know the answer but refuse to elaborate.",
	"Your future contains an expertly timed high five.",
	"A patient turtle will remind you that speed is overrated.",
	"An ordinary evening will end with an extraordinary laugh.",
	"The stars predict an encounter with a majestic cardboard box.",
	"You will soon defeat an enemy known only as Mild Inconvenience.",
	"A very serious goose will grant you honorary membership.",
	"Your next great plan will begin with the words 'what if'.",
	"A wandering breeze will deliver a compliment from yesterday.",
	"The cosmos has accepted your application for additional whimsy.",
	"Your tomorrow will contain a small but satisfying victory.",
	"The universe is proud of you, though it refuses to say why.",
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
