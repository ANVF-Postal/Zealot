package main

// messageIDAt returns the ID of the user-authored message under a screen
// coordinate. It maps the visible viewport back to the flattened render cache,
// so wrapped messages and scroll offsets resolve to the same message ID.
func (m model) messageIDAt(x, y int) string {
	if m.usersOpen || m.splashOpen || m.width < 1 {
		return ""
	}
	left := 0
	if !narrowBordersHidden(m.width) {
		left = 1 // exclude the chat pane's left border
	}
	if x < left || x >= m.chatScrollbarX() {
		return ""
	}

	chatHeight := max(1, m.messageHeight()-1) // typing indicator has its own row
	if y < 2 || y >= 2+chatHeight {
		return ""
	}

	lines := m.renderMessageLines()
	start := len(lines) - chatHeight - m.scroll
	if start < 0 {
		start = 0
	}
	row := start + y - 2
	if row < 0 || row >= len(lines) {
		return ""
	}

	ids := m.renderedMessageIDs
	if !m.renderCacheValid || len(ids) != len(lines) {
		ids = make([]string, 0, len(lines))
		for _, packet := range m.messages {
			for range m.renderPacketLines(packet, m.width) {
				ids = append(ids, packet.ID)
			}
		}
	}
	if row >= len(ids) || ids[row] == "" {
		return ""
	}

	id := ids[row]
	for _, packet := range m.messages {
		if packet.ID == id && packet.Type == "message" {
			return id
		}
	}
	return ""
}
