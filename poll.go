package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Votes are owned by the daemon, not by client-submitted counters.
// A poll remains available until the room is destroyed. Voter IDs are the
// canonical room usernames, so reconnecting with the same name retains a vote.
type pollState struct {
	id string
	version int
	creator string
	creatorID string
	author User
	question string
	options []string
	votes map[string][]int // canonical lowercase name -> selected one-based choices
	multi bool
	closed bool
	time time.Time
	messageID string
}

func parsePollDefinition(input string) (string, []string, error) {
	parts := strings.Split(input, "|")
	if len(parts) < 3 || len(parts) > 7 {
		return "", nil, fmt.Errorf("Usage: /poll question | option1 | option2 (2–6 options)")
	}
	question := strings.TrimSpace(parts[0])
	if question == "" || len([]rune(question)) > 160 || strings.ContainsAny(question, "\r\n\t") {
		return "", nil, fmt.Errorf("Poll question must contain 1–160 characters")
	}
	options := make([]string, 0, len(parts)-1)
	seen := make(map[string]bool)
	for _, raw := range parts[1:] {
		option := strings.TrimSpace(raw)
		if option == "" || len([]rune(option)) > 60 || strings.ContainsAny(option, "\r\n\t") {
			return "", nil, fmt.Errorf("Poll options must contain 1–60 characters")
		}
		key := strings.ToLower(option)
		if seen[key] {
			return "", nil, fmt.Errorf("Poll options must be distinct")
		}
		seen[key] = true
		options = append(options, option)
	}
	return question, options, nil
}

func (p *pollState) snapshot(name, roomName string) Packet {
	choices := p.votes[strings.ToLower(name)]
	choice := 0
	if len(choices)>0 {choice=choices[0]}
	data := &PollData{
		ID: p.id, Version: p.version, Question: p.question,
		Options: append([]string(nil), p.options...),
		Closed: p.closed, Choice: choice, Choices: append([]int(nil),choices...), Multi: p.multi,
		ResultsVisible: len(choices)>0 || p.closed,
	}
	if data.ResultsVisible {
		data.Counts = make([]int, len(p.options))
		for _, choices := range p.votes {
			for _, selected := range choices {if selected>0 && selected<=len(data.Counts) {data.Counts[selected-1]++}}
		}
		data.Total = len(p.votes)
	}
	return Packet{
		Type: "poll_result", ID: p.messageID, Room: roomName,
		User: p.author, Time: p.time, Poll: data,
	}
}

func (s *server) notifyPoll(roomName string, id string) {
	s.mu.Lock()
	room := s.rooms[roomName]
	if room == nil || room.polls[id] == nil {
		s.mu.Unlock()
		return
	}
	poll := room.polls[id]
	type delivery struct { client *serverClient; packet Packet }
	batch := make([]delivery, 0, len(room.clients))
	for _, client := range room.clients {
		batch = append(batch, delivery{client, poll.snapshot(client.user.Name, roomName)})
	}
	s.mu.Unlock()
	for _, d := range batch {
		d.client.send(d.packet)
	}
}

func (s *server) createPoll(roomName string, sender *serverClient, text string, multi bool) {
	question, options, err := parsePollDefinition(text)
	if err != nil {
		sender.send(Packet{Type:"system",Text:err.Error(),Time:time.Now().UTC()})
		return
	}
	s.mu.Lock()
	room := s.rooms[roomName]
	if room == nil || room.clients[sender.id] != sender {
		s.mu.Unlock()
		return
	}
	room.pollSequence++
	id := strconv.Itoa(room.pollSequence)
	state := &pollState{
		id:id, version:1, creator:sender.user.Name, creatorID:sender.id,
		author:sender.user, question:question, options:options,
		votes:make(map[string][]int), multi:multi, time:time.Now().UTC(),
		messageID:nextMessageID(),
	}
	room.polls[id] = state
	if room.historyEnabled {
		// Replay is personalized for each newly joining client.
		room.messages = append(room.messages, state.snapshot("", roomName))
	}
	s.mu.Unlock()
	s.notifyPoll(roomName,id)
}

func (s *server) votePoll(roomName string, sender *serverClient, request *PollData) {
	if request == nil { sender.send(Packet{Type:"system",Text:"Usage: /vote POLL_ID OPTION_NUMBER",Time:time.Now().UTC()}); return }
	s.mu.Lock()
	room := s.rooms[roomName]
	if room == nil || room.clients[sender.id] != sender {
		s.mu.Unlock()
		return
	}
	poll := room.polls[request.ID]
	if poll == nil || poll.closed || request.Choice < 1 || request.Choice > len(poll.options) {
		s.mu.Unlock()
		sender.send(Packet{Type:"system",Text:"Poll not found, closed, or invalid option.",Time:time.Now().UTC()})
		return
	}
	name:=strings.ToLower(sender.user.Name)
	if poll.multi {
		old:=poll.votes[name]
		selected:=make([]int,0,len(old)+1)
		found:=false
		for _,option:=range old {
			if option==request.Choice {found=true;continue}
			selected=append(selected,option)
		}
		if !found {selected=append(selected,request.Choice)}
		if len(selected)==0 {delete(poll.votes,name)} else {poll.votes[name]=selected}
	} else {
		poll.votes[name]=[]int{request.Choice}
	}
	poll.version++
	s.mu.Unlock()
	s.notifyPoll(roomName,request.ID)
}

func (s *server) closePoll(roomName string, sender *serverClient, id string) {
	s.mu.Lock()
	room := s.rooms[roomName]
	if room == nil || room.clients[sender.id] != sender {
		s.mu.Unlock()
		return
	}
	poll := room.polls[strings.TrimSpace(id)]
	if poll == nil || (poll.creatorID != sender.id && !strings.EqualFold(poll.creator, sender.user.Name)) {
		s.mu.Unlock()
		sender.send(Packet{Type:"system",Text:"Only the poll creator can close an existing poll.",Time:time.Now().UTC()})
		return
	}
	if poll.closed {
		s.mu.Unlock()
		return
	}
	poll.closed = true
	poll.version++
	id = poll.id
	s.mu.Unlock()
	s.notifyPoll(roomName,id)
}

// historyPolls produces the recipient's own snapshots. A reconnecting voter
// can see results again, but a new arrival sees only options until they vote.
func (s *server) personalizePollHistory(room *roomState, username, roomName string) []Packet {
	result := make([]Packet, 0, len(room.messages)+len(room.polls))
	seen := make(map[string]bool)
	for _, item := range room.messages {
		if item.Type == "poll_result" && item.Poll != nil {
			if poll := room.polls[item.Poll.ID]; poll != nil {
				result = append(result, poll.snapshot(username,roomName))
				seen[poll.id] = true
			}
		} else {
			result = append(result, item)
		}
	}
	// Rooms without history still surface currently open polls to newcomers.
	if !room.historyEnabled {
		for n:=1; n<=room.pollSequence; n++ {
			id:=strconv.Itoa(n)
			if poll:=room.polls[id]; poll!=nil && !poll.closed && !seen[id] {
				result=append(result,poll.snapshot(username,roomName))
			}
		}
	}
	return result
}
