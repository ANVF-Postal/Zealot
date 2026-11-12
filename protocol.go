package main

import (
	"time"
)

const (
	socketPath = "/tmp/zealot.sock"
	maxMessage = 4000
)

// -----------------------------------------------------------------------------
// Wire protocol
// -----------------------------------------------------------------------------

type User struct {
	Name   string `json:"name"`
	Color  string `json:"color"`
	Typing bool   `json:"typing,omitempty"`
}

// PollData is a per-viewer snapshot. Vote totals are withheld until that
// viewer has voted or the poll has closed. Choice is one-based (zero = none).
type PollData struct {
	ID string `json:"id"`
	Version int `json:"version"`
	Question string `json:"question"`
	Options []string `json:"options"`
	Counts []int `json:"counts,omitempty"`
	Total int `json:"total"`
	Choice int `json:"choice,omitempty"`
	Choices []int `json:"choices,omitempty"`
	Multi bool `json:"multi,omitempty"`
	Closed bool `json:"closed,omitempty"`
	ResultsVisible bool `json:"results_visible,omitempty"`
}

type Packet struct {
	Type       string    `json:"type"`
	ID         string    `json:"id,omitempty"`
	ReplyTo    string    `json:"reply_to,omitempty"`
	Room       string    `json:"room,omitempty"`
	User       User      `json:"user,omitempty"`
	Text       string    `json:"text,omitempty"`
	FortuneTo  string    `json:"fortune_to,omitempty"`
	WhisperTo  string    `json:"whisper_to,omitempty"`
	Poll       *PollData `json:"poll,omitempty"`
	Theme      *ThemePreset `json:"theme,omitempty"`
	Mentions   []string  `json:"mentions,omitempty"`
	Time       time.Time `json:"time,omitempty"`
	Typing     bool      `json:"typing,omitempty"`
	AddHistory bool      `json:"add_history,omitempty"`
	Ephemeral   bool      `json:"ephemeral,omitempty"`
	Rainbow     bool      `json:"rainbow,omitempty"`
	RainbowPhase float64   `json:"rainbow_phase,omitempty"`
	XKeyscore    int       `json:"xkeyscore,omitempty"`
	Long         bool      `json:"long,omitempty"`
	Spoiler      bool      `json:"spoiler,omitempty"`
	Users       []User    `json:"users,omitempty"`
}
