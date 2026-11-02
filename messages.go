package main

import (
	"os"

	tea "charm.land/bubbletea/v2"
)

func terminalBellCmd() tea.Cmd {
	return func() tea.Msg {
		_, _ = os.Stdout.Write([]byte{'\a'})
		return nil
	}
}

// -----------------------------------------------------------------------------
// UI messages
// -----------------------------------------------------------------------------

type messageReceivedMsg Packet

type historyBatchReceivedMsg struct {
	Packets  []Packet
	Complete bool
}

type connectionClosedMsg struct{}

type typingClearMsg struct {
	generation int
}

type notificationsReadyMsg struct{}

type quitConfirmTimeoutMsg struct {
	generation int
}

type sendFeedbackTimeoutMsg struct {
	generation int
}

type scrollbarHoldMsg struct{ generation int }
type usersScrollbarHoldMsg struct{ generation int }
type splashScrollbarHoldMsg struct{ generation int }
type usersAnimationMsg struct{}
