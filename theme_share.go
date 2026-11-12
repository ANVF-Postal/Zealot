package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	styled := muted.Render("["+packet.Time.Local().Format("15:04")+"] ") +
		nameStyle.Render(terminalSafeText(packet.User.Name)) + normal.Render(" sent a ") + accent.Render("[Theme]") + normal.Render("!")
	var result []string
	rowFill := lipgloss.NewStyle().Width(w).Background(bg)
	for _, line := range wrapANSIHardLine(styled, w) {
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
	// Compute the same wrapped label position used by renderThemeShareLines.
	prefix := "["+pkt.Time.Local().Format("15:04")+"] "+terminalSafeText(pkt.User.Name)+" sent a "
	contentWidth := max(1, chatContentWidth(m.width))
	start := lipgloss.Width(prefix)
	// wrapANSIHardLine uses terminal cells; calculate the visible label
	// position by scanning the already rendered row rather than guessing.
	rel := row-span.Start
	rendered := m.renderThemeShareLines(pkt, m.width)
	if rel < 0 || rel >= len(rendered) { return Packet{}, false }
	plain := stripANSISequences(rendered[rel])
	startPos := strings.Index(plain, "[Theme]")
	if startPos < 0 && start >= contentWidth { return Packet{}, false }
	if startPos >= 0 && x-left >= startPos && x-left < startPos+len("[Theme]") {
		return pkt, true
	}
	return Packet{}, false
}

// Keep os.ErrExist in scope to make the O_EXCL contract explicit.
var _ = errors.Is
var _ = filepath.Base
