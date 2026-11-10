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

type Packet struct {
	Type       string    `json:"type"`
	ID         string    `json:"id,omitempty"`
	ReplyTo    string    `json:"reply_to,omitempty"`
	Room       string    `json:"room,omitempty"`
	User       User      `json:"user,omitempty"`
	Text       string    `json:"text,omitempty"`
	FortuneTo  string    `json:"fortune_to,omitempty"`
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
