package main

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

// Shared themes contain palette data only: no paths, scripts, config files,
// external references or other executable content are accepted.
const maxSharedThemeName = 80
const maxSharedThemeDescription = 500

func validSharedThemeText(s string, limit int, required bool) bool {
	if !utf8.ValidString(s) || len([]rune(s)) > limit { return false }
	if required && strings.TrimSpace(s) == "" { return false }
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) { return false }
	}
	return true
}

func validateSharedTheme(p *ThemePreset) error {
	if p == nil { return fmt.Errorf("missing theme data") }
	// The ID is local metadata; never accept an arbitrary path or filename.
	if p.ID != "" {
		if len(p.ID) > 80 { return fmt.Errorf("invalid theme identifier") }
		for _, c := range p.ID {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
				return fmt.Errorf("invalid theme identifier")
			}
		}
	}
	if !validSharedThemeText(p.Name, maxSharedThemeName, true) {
		return fmt.Errorf("invalid theme name")
	}
	if !validSharedThemeText(p.Description, maxSharedThemeDescription, false) {
		return fmt.Errorf("invalid theme description")
	}
	if err := validateThemeColors(p.Colors); err != nil { return err }
	// validateThemeColors accepts whitespace-trimmed colors for local legacy
	// configuration. Network themes must contain exact canonical values to
	// avoid persisting control/whitespace bytes in trusted theme files.
	for _, value := range []string{
		p.Colors.Background, p.Colors.TextColor, p.Colors.BorderColor,
		p.Colors.AccentColor, p.Colors.HoverTextColor, p.Colors.MutedTextColor,
		p.Colors.MentionBackgroundColor, p.Colors.SelectionBackgroundColor,
	} {
		if value != strings.TrimSpace(value) || !utf8.ValidString(value) {
			return fmt.Errorf("theme colors must be plain hex or ANSI values")
		}
	}
	return nil
}

// installSharedTheme never trusts a filename or ID supplied over the wire.
// It only writes an exclusively-created TOML file inside the themes directory.
// A collision uses theme-name1.toml, theme-name2.toml, etc.
func installSharedTheme(received ThemePreset) (themeListEntry, error) {
	if err := validateSharedTheme(&received); err != nil { return themeListEntry{}, err }
	if err := ensureThemeLibrary(); err != nil { return themeListEntry{}, err }
	if err := os.MkdirAll(themesDir(), 0700); err != nil { return themeListEntry{}, err }
	stem := themeFilenameStem(received.Name)
	for suffix := 0; suffix < 10000; suffix++ {
		filename := stem + ".toml"
		if suffix > 0 { filename = fmt.Sprintf("%s%d.toml", stem, suffix) }
		received.ID = strings.TrimPrefix(strings.TrimSuffix(filename, ".toml"), "theme-")
		pack := ThemePack{SchemaVersion: themePackSchemaVersion, PackName: "Shared themes", Themes: []ThemePreset{received}}
		err := saveThemePack(filename, pack)
		if err == nil {
			return themeListEntry{Preset: received, PackName: pack.PackName, ThemeCount: 1, FileName: filename}, nil
		}
		// Exclusive O_EXCL creation in saveThemePack prevents races as well.
		if !strings.Contains(err.Error(), "already exists") { return themeListEntry{}, err }
		// A duplicate name should not overwrite an existing file.
	}
	return themeListEntry{}, fmt.Errorf("no available filename for shared theme")
}

func (s *server) shareTheme(roomName string, sender *serverClient, preset *ThemePreset) {
	if err := validateSharedTheme(preset); err != nil {
		sender.send(Packet{Type: "system", Room: roomName, Text: "Theme rejected: "+err.Error(), Time: time.Now().UTC()})
		return
	}
	s.mu.Lock()
	room := s.rooms[roomName]
	allowed := room != nil && room.clients[sender.id] == sender && !sender.ephemeral
	s.mu.Unlock()
	if !allowed {
		sender.send(Packet{Type: "system", Room: roomName, Text: "Theme sharing is not available to this client.", Time: time.Now().UTC()})
		return
	}
	// Copy validated data and derive every authoritative packet field on the server.
	clean := *preset
	msg := Packet{ID: nextMessageID(), Type: "theme_result", Room: roomName, User: sender.user, Theme: &clean, Time: time.Now().UTC()}
	s.mu.Lock()
	if room := s.rooms[roomName]; room != nil && room.historyEnabled {
		room.messages = append(room.messages, msg)
	}
	s.mu.Unlock()
	s.broadcast(roomName, msg)
}

// renderThemeShareLines is also the layout source for theme link hit testing.
func (m model) renderThemeShareLines(packet Packet, width int) []string {
	w := max(1, chatContentWidth(width))
	bg := m.messageBackground(packet)
	normal := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.TextColor)).Background(bg)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.MutedTextColor)).Background(bg)
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.AccentColor)).Background(bg).Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(packet.User.Color)).Background(bg).Bold(true)
	prefix := muted.Render("["+packet.Time.Local().Format("15:04")+"] ") +
		nameStyle.Render(terminalSafeText(packet.User.Name)) + normal.Render(" sent a ")
	link := accent.Render("[Theme]") + normal.Render("!")
	var result []string
	rowFill := lipgloss.NewStyle().Width(w).Background(bg)
	var wrapped []string
	if lipgloss.Width(prefix)+lipgloss.Width(link) <= w {
		wrapped = wrapANSIHardLine(prefix+link,w)
	} else {
		// Never split the clickable token across two terminal rows.
		wrapped = append(wrapped, wrapANSIHardLine(prefix,w)...)
		wrapped = append(wrapped, wrapANSIHardLine(link,w)...)
	}
	for _, line := range wrapped {
		result = append(result, renderChatRow(rowFill, line, w))
	}
	return result
}

func (m model) sharedThemeLinkAt(x, y int) (Packet, bool) {
	if m.usersOpen || m.splashOpen || m.width < 1 { return Packet{}, false }
	left := 0
	if !narrowBordersHidden(m.width) { left = 1 }
	if x < left || x >= m.chatScrollbarX() { return Packet{}, false }
	height := max(1, m.messageHeight()-1)
	if y < 2 || y >= 2+height { return Packet{}, false }
	lines := m.renderMessageLines()
	row := max(0, len(lines)-height-m.scroll)+y-2
	pkt, span, ok := m.messageAtRow(row)
	if !ok || pkt.Type != "theme_result" || pkt.Theme == nil { return Packet{}, false }
	if validateSharedTheme(pkt.Theme) != nil { return Packet{}, false }
	// Use the rendered row to locate the link, even after a wrap or
	// with multicolumn Unicode characters in the sender's display name.
	rel := row-span.Start
	rendered := m.renderThemeShareLines(pkt, m.width)
	if rel < 0 || rel >= len(rendered) { return Packet{}, false }
	plain := stripANSISequences(rendered[rel])
	startPos := strings.Index(plain, "[Theme]")
	if startPos >= 0 {
		startCell := lipgloss.Width(plain[:startPos])
		if x-left >= startCell && x-left < startCell+lipgloss.Width("[Theme]") {
			return pkt, true
		}
	}
	return Packet{}, false
}


func (m *model) beginThemeShare() {
	entry, ok := m.selectedThemeEntry()
	if !ok || !themeEntryEditable(entry) || !m.connected || m.client == nil || m.themeDirty || m.themeEditField != "" {
		m.themeEditError = "Only saved custom themes can be shared while connected. Clone a built-in theme first."
		return
	}
	if err := validateSharedTheme(&entry.Preset); err != nil {
		m.themeEditError = "Cannot share this theme: "+err.Error()
		return
	}
	m.themeShareConfirm = true
	m.themeDetailsScroll = 0
	m.themeEditError = ""
}

func (m *model) confirmThemeShare() {
	if !m.themeShareConfirm { return }
	entry, ok := m.selectedThemeEntry()
	if !ok || !themeEntryEditable(entry) || !m.connected || m.client == nil || m.themeDirty {
		m.themeEditError = "Cannot share this theme."
		m.themeShareConfirm = false
		return
	}
	if err := validateSharedTheme(&entry.Preset); err != nil {
		m.themeEditError = "Cannot share: "+err.Error()
		m.themeShareConfirm = false
		return
	}
	preset := entry.Preset
	if err := m.client.send(Packet{Type: "theme_share", Room: m.room, Theme: &preset}); err != nil {
		m.themeEditError = "Could not send theme: "+err.Error()
		m.themeShareConfirm = false
		return
	}
	m.themeShareConfirm = false
	m.themesOpen = false
	m.splashOpen = false
}

func (m *model) openReceivedTheme(packet Packet) {
	if packet.Type != "theme_result" || validateSharedTheme(packet.Theme) != nil { return }
	m.openThemeBrowser()
	theme := *packet.Theme
	m.themeIncoming = &theme
	m.themeIncomingSender = terminalSafeText(packet.User.Name)
	m.themeDetailsScroll = 0
	m.themeError = ""
	m.themeEditError = ""
}

func (m *model) closeReceivedTheme() {
	m.themeIncoming = nil
	m.themeIncomingSender = ""
	m.themesOpen = false
	m.splashOpen = false
	m.themeError = ""
	m.themeEditError = ""
}

func (m *model) installReceivedTheme() {
	if m.themeIncoming == nil { return }
	entry, err := installSharedTheme(*m.themeIncoming)
	if err != nil {
		m.themeEditError = "Install failed: "+err.Error()
		m.themeDetailsScroll = m.maxThemeDetailsScroll()
		return
	}
	m.themeIncoming = nil
	m.themeIncomingSender = ""
	m.reloadThemeEntries(entry.FileName,entry.Preset.ID)
	m.themesOpen = false
	m.splashOpen = false
	m.appendMessage(Packet{Type:"system",Room:m.room,Text:"Installed theme "+entry.Preset.Name+" as "+entry.FileName+". Open Themes to apply it.",Time:time.Now().UTC()})
}
