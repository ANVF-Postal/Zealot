package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// pollLayout is the single layout definition used by rendering and mouse
// hit-testing. Each row in choiceRows stores its one-based option (or zero).
// It computes only the visible poll, never the whole message history.
func (m model) pollLayout(packet Packet, width int) ([]string, []int) {
	if packet.Poll == nil {
		return []string{"[POLL] Invalid poll"}, []int{0}
	}
	p := packet.Poll
	w := max(1, chatContentWidth(width))
	rows := make([]string, 0, len(p.Options)+8)
	options := make([]int, 0, len(p.Options)+8)
	add := func(text string, choice int) {
		rows = append(rows, truncateANSI(text,w))
		options = append(options,choice)
	}
	header := fmt.Sprintf("[%s] [POLL #%s] %s asks:", packet.Time.Local().Format("15:04"),p.ID,terminalSafeText(packet.User.Name))
	for _, line := range wrapANSIHardLine(header,w) { add(line,0) }
	for _, line := range wrapANSIHardLine(terminalSafeText(p.Question),w) { add(line,0) }
	for i, option := range p.Options {
		choice := i+1
		lead := fmt.Sprintf("  %d. ",choice)
		if p.Choice == choice {
			lead = fmt.Sprintf("> %d. ",choice)
		}
		suffix := ""
		if p.ResultsVisible {
			count := 0
			if i < len(p.Counts) {count=p.Counts[i]}
			percent := 0
			if p.Total > 0 { percent = (count*100+p.Total/2)/p.Total }
			if width > 48 && w >= 40 {
				filled := 0
				if p.Total > 0 {filled = count*12/p.Total}
				suffix = fmt.Sprintf(" %s%s %d%%",
					strings.Repeat("█",filled),strings.Repeat("░",12-filled),percent)
			} else {
				suffix = fmt.Sprintf(" %d%%",percent)
			}
		}
		available := max(1,w-lipgloss.Width(lead)-lipgloss.Width(suffix))
		label := truncateANSI(terminalSafeText(option),available)
		add(lead+label+suffix,choice)
	}
	if p.Closed {
		best, winners := -1, make([]string,0,len(p.Options))
		for i, count := range p.Counts {
			if count>best {
				best=count
				winners=[]string{p.Options[i]}
			} else if count==best {
				winners=append(winners,p.Options[i])
			}
		}
		verdict := "No votes"
		if best > 0 {
			if len(winners)==1 {verdict="Winner: "+winners[0]} else {verdict="Tie: "+strings.Join(winners,", ")}
		}
		for _, line := range wrapANSIHardLine(fmt.Sprintf("%d votes • Closed • %s",p.Total,verdict),w) {add(line,0)}
	} else if p.ResultsVisible {
		add(fmt.Sprintf("%d votes • Open • /vote %s OPTION",p.Total,p.ID),0)
	} else {
		add(fmt.Sprintf("Vote to reveal results • /vote %s OPTION",p.ID),0)
	}
	return rows,options
}

func (m model) renderPollLines(packet Packet, width int) []string {
	rows,choices := m.pollLayout(packet,width)
	bg := m.messageBackground(packet)
	w := chatContentWidth(width)
	rowFill := lipgloss.NewStyle().Width(w).Background(bg)
	plain := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.TextColor)).Background(bg)
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.AccentColor)).Background(bg).Bold(true)
	highlight := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.HoverTextColor)).Background(bg)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.MutedTextColor)).Background(bg)
	rendered := make([]string,len(rows))
	headerRows := []string(nil)
	if packet.Poll != nil {
		nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(packet.User.Color)).Background(bg).Bold(true)
		styledHeader := muted.Render("["+packet.Time.Local().Format("15:04")+"] ") +
			accent.Render("[POLL #"+packet.Poll.ID+"] ") +
			nameStyle.Render(terminalSafeText(packet.User.Name)) +
			plain.Render(" asks:")
		headerRows = wrapANSIHardLine(styledHeader,max(1,w))
	}
	for i,line := range rows {
		if i<len(headerRows) {
			rendered[i]=renderChatRow(rowFill,headerRows[i],w)
			continue
		}
		style := plain
		switch {
		case choices[i]>0 && m.pollHoveredID==packet.ID && m.pollHoveredOption==choices[i]:
			style=highlight
		case choices[i]>0 && packet.Poll != nil && packet.Poll.Choice==choices[i]:
			style=accent
		case i==len(rows)-1:
			style=muted
		}
		rendered[i]=renderChatRow(rowFill,style.Render(line),w)
	}
	return rendered
}

// applyPollSnapshot updates just the original poll in the message cache. The
// server sends snapshots, not new chat lines, on every vote and close.
func (m *model) applyPollSnapshot(packet Packet) {
	if packet.Poll == nil || packet.ID == "" {return}
	index, exists := m.pollIndexByID[packet.ID]
	if !exists || index<0 || index>=len(m.messages) {
		// Older models or direct tests may not have an index yet.
		for i, previous := range m.messages {
			if previous.ID == packet.ID && previous.Type == "poll_result" {
				index, exists = i, true
				break
			}
		}
	}
	if !exists || m.messages[index].Type != "poll_result" {
		m.appendMessage(packet)
		return
	}
	if old := m.messages[index].Poll; old != nil && old.Version >= packet.Poll.Version {
		return
	}
	m.messages[index] = packet
	if m.pollIndexByID == nil {m.pollIndexByID = make(map[string]int)}
	m.pollIndexByID[packet.ID] = index
	if m.messageByID != nil {m.messageByID[packet.ID] = packet}
	if m.renderCacheValid && m.renderCacheWidth == m.width {
		oldRows := m.messageRowsByID[packet.ID]
		if _, ok := m.replaceCachedMessageRows(packet.ID); ok {
			newRows := m.messageRowsByID[packet.ID]
			if m.scroll > 0 {
				m.scroll += (newRows.End-newRows.Start)-(oldRows.End-oldRows.Start)
			}
			m.clampScroll()
			m.refreshRenderedWindow()
			return
		}
	}
	m.rebuildMessageCache()
}

// pollOptionAt maps a mouse position to one of the visible poll's option
// rows; unlike normal message text hitboxes, the full option row is clickable.
func (m model) pollOptionAt(x,y int) (string,int) {
	if m.usersOpen || m.splashOpen || m.width<1 {return "",0}
	left:=0
	if !narrowBordersHidden(m.width) {left=1}
	if x<left || x>=m.chatScrollbarX() {return "",0}
	height:=max(1,m.messageHeight()-1)
	if y<2 || y>=2+height {return "",0}
	lines:=m.renderMessageLines()
	start:=max(0,len(lines)-height-m.scroll)
	row:=start+y-2
	packet,span,ok:=m.messageAtRow(row)
	if !ok || packet.Type!="poll_result" || packet.Poll==nil || packet.Poll.Closed {return "",0}
	_,choices:=m.pollLayout(packet,m.width)
	relative:=row-span.Start
	if relative<0 || relative>=len(choices) || choices[relative]==0 {return "",0}
	return packet.ID,choices[relative]
}
