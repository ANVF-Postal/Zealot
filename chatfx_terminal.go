package main

import (
    "fmt"
    "strings"
    "time"
    "unicode/utf8"

    "charm.land/lipgloss/v2"
)

// Terminal is intentionally independent of the primary background effect.
// A large future deadline represents an untimed room-wide session.
var terminalForever=time.Date(9999,1,1,0,0,0,0,time.UTC)

const (
    terminalLetterDelay=38*time.Millisecond
    terminalPeriodPause=250*time.Millisecond
    terminalCursorBlink=500*time.Millisecond
)

type terminalReveal struct {
    started time.Time
    text string
}

func terminalSentencePause(ch rune) bool {
    return ch=='.' || ch=='?' || ch=='!'
}

// A simple Commodore-inspired solid/hollow square. Blink phase is based on
// time, not frame count, so /fxfps never changes the blink rate.
func terminalCursorGlyph(now time.Time) rune {
    if (now.UnixNano()/int64(terminalCursorBlink))%2==0 {return '█'}
    return '□'
}

func terminalTextDuration(text string) time.Duration {
    duration:=time.Duration(0)
    for _,ch:=range text {
        duration+=terminalLetterDelay
        if terminalSentencePause(ch) {duration+=terminalPeriodPause}
    }
    return duration
}

func terminalRevealCount(text string,elapsed time.Duration) int {
    if elapsed<0 {return 0}
    count:=0
    for _,ch:=range text {
        if elapsed<terminalLetterDelay {return count}
        elapsed-=terminalLetterDelay
        count++
        if terminalSentencePause(ch) {
            if elapsed<terminalPeriodPause {return count}
            elapsed-=terminalPeriodPause
        }
    }
    return count
}

func (m model) terminalEnabled() bool {
    if m.fxDisabled {return false}
    now:=time.Now()
    local:=!m.terminalLocalUntil.IsZero() && now.Before(m.terminalLocalUntil)
    room:=!m.terminalRoomUntil.IsZero() && now.Before(m.terminalRoomUntil)
    return local || room
}

func (m *model) terminalTrackMessage(packet Packet) {
    if !m.terminalEnabled() {return}
    // Type newly arrived chat, actions and system messages, but not old
    // history, rich interactive artwork, polls, or command/result UI.
    if packet.Type!="message" && packet.Type!="whisper_result" &&
        packet.Type!="system" && packet.Type!="me" {return}
    if packet.XKeyscore>0 || packet.Long || packet.Text=="" {return}
    if m.terminalReveals==nil {m.terminalReveals=make(map[int]terminalReveal)}
    m.terminalReveals[len(m.messages)-1]=terminalReveal{
        // Hard line breaks reserve another row rather than consuming a
        // printable-character tick. The original packet remains untouched.
        started:time.Now(),text:strings.ReplaceAll(strings.ReplaceAll(packet.Text,"\r",""),"\n",""),
    }
}

func (m *model) terminalAdvance(now time.Time) {
    if len(m.terminalReveals)==0 {return}
    if !m.terminalEnabled() {
        clear(m.terminalReveals)
        return
    }
    latest:=m.terminalLatestMessage()
    for index,reveal:=range m.terminalReveals {
        if index!=latest && now.Sub(reveal.started)>=terminalTextDuration(reveal.text) {
            delete(m.terminalReveals,index)
        }
    }
}

// The cursor belongs only to the newest incoming message, not every
// partially typed message. Its completed reveal remains tracked indefinitely
// while /terminal is enabled, so the cursor can keep blinking in place.
func (m model) terminalLatestMessage() int {
    latest:=-1
    for index:=range m.terminalReveals {
        if index>latest {latest=index}
    }
    return latest
}

func (m model) terminalIsTyping(now time.Time) bool {
    for _,reveal:=range m.terminalReveals {
        if now.Sub(reveal.started)<terminalTextDuration(reveal.text) {
            return true
        }
    }
    return false
}

// Paint the *already wrapped* full message, hiding only body glyphs that
// haven't been typed yet. This preserves stable row counts, original
// colours, hover/reply geometry, ANSI escapes and the scrollbar position.
// visibleCount is shared across all rows of one message.
// Mask unrevealed body characters without altering the row's terminal width.
// When this is the newest message, the block occupies the first hidden cell.
func terminalMaskRow(row string,body messageBodyRange,visibleCount *int,cursorPlaced *bool,cursor rune) string {
    if body.End<=body.Start {return row}
    var out strings.Builder
    out.Grow(len(row)+32)
    column:=0
    for i:=0;i<len(row); {
        if row[i]==0x1b {
            end:=ansiSequenceEnd(row,i)
            if end<=i {end=i+1}
            out.WriteString(row[i:end])
            i=end
            continue
        }
        ch,n:=utf8.DecodeRuneInString(row[i:])
        if n<=0 {break}
        width:=lipgloss.Width(string(ch))
        if width<0 {width=0}
        inside:=column>=body.Start && column<body.End
        if inside {
            if *visibleCount>0 {
                out.WriteString(row[i:i+n])
                *visibleCount--
            } else if !*cursorPlaced && cursor!=0 {
                out.WriteString("\x1b[7m"+string(cursor)+"\x1b[27m")
                if width>1 {out.WriteString(strings.Repeat(" ",width-1))}
                *cursorPlaced=true
            } else {
                out.WriteString(strings.Repeat(" ",width))
            }
        } else {
            out.WriteString(row[i:i+n])
        }
        column+=width
        i+=n
    }
    return out.String()
}

// Place the persistent cursor in a spare cell immediately after the final
// body glyph, never append another cell and never alter the reserved row count.
func terminalCursorAtEnd(row string,column int,cursor rune) string {
    if column<0 {return row}
    var out strings.Builder
    out.Grow(len(row)+24)
    pos:=0
    painted:=false
    for i:=0;i<len(row); {
        if row[i]==0x1b {
            end:=ansiSequenceEnd(row,i)
            if end<=i {end=i+1}
            out.WriteString(row[i:end])
            i=end
            continue
        }
        ch,size:=utf8.DecodeRuneInString(row[i:])
        if size<=0 {break}
        width:=lipgloss.Width(string(ch))
        if !painted && pos==column && width==1 {
            out.WriteString("\x1b[7m"+string(cursor)+"\x1b[27m")
            painted=true
        } else {
            out.WriteString(row[i:i+size])
        }
        pos+=width
        i+=size
    }
    return out.String()
}

// Locate message content without ever altering the original packet, so the
// layout does not contract while typing (including narrow/compact layouts).
func (m model) terminalBodyRanges(packet Packet,lines []string) []messageBodyRange {
    if packet.Type=="message" || packet.Type=="whisper_result" {
        return m.messageBodyRangesForLineCount(packet,m.width,len(lines))
    }
    ranges:=make([]messageBodyRange,len(lines))
    contentWidth:=chatContentWidth(m.width)
    prefix:=fmt.Sprintf("[%s] ",packet.Time.Local().Format("15:04"))
    if packet.Type=="me" {
        prefix=fmt.Sprintf("[%s] * %s ",packet.Time.Local().Format("15:04"),packet.User.Name)
    }
    startFirst:=2+lipgloss.Width(prefix)
    for i,line:=range lines {
        start:=2
        if i==0 {start=startFirst}
        used:=lipgloss.Width(strings.TrimRight(stripANSISequences(line)," "))
        if start<used {
            ranges[i]=clippedMessageBodyRange(start,used,contentWidth)
        }
    }
    return ranges
}

func (m model) renderTerminalRows(visible []string,globalStart int) {
    if len(m.terminalReveals)==0 || !m.terminalEnabled() || !m.renderCacheValid {
        return
    }
    now:=time.Now()
    for index,reveal:=range m.terminalReveals {
        if index<0 || index>=len(m.messageRowRanges) || index>=len(m.messages) {continue}
        rows:=m.messageRowRanges[index]
        if rows.Start>=globalStart+len(visible) || rows.End<=globalStart {continue}
        packet:=m.messages[index]
        if packet.Type!="message" && packet.Type!="whisper_result" &&
            packet.Type!="system" && packet.Type!="me" {continue}
        elapsed:=now.Sub(reveal.started)
        if elapsed>=terminalTextDuration(reveal.text) {continue}
        remaining:=terminalRevealCount(reveal.text,elapsed)
        // Offscreen cache rows may be intentionally blank. Reconstruct the
        // message once to keep character progress accurate when scrolled.
        fullLines:=m.renderPacketLines(packet,m.width)
        ranges:=m.terminalBodyRanges(packet,fullLines)
        placed:=false
        // Consume all prior rows, even when scrolled halfway through a
        // long message. The reveal remains continuous across wrapping.
        for row:=rows.Start;row<rows.End;row++ {
            relative:=row-rows.Start
            if relative>=len(ranges) {break}
            if row<globalStart || row>=globalStart+len(visible) {
                // Count real displayed body runes in omitted previous rows.
                if row<globalStart && relative<len(fullLines) {
                    line:=stripANSISequences(fullLines[relative])
                    col:=0
                    for _,r:=range line {
                        width:=lipgloss.Width(string(r))
                        if col>=ranges[relative].Start && col<ranges[relative].End {
                            if remaining>0 {remaining--} else if !placed {placed=true}
                        }
                        col+=width
                    }
                }
                continue
            }
            visible[row-globalStart]=terminalMaskRow(
                visible[row-globalStart],ranges[relative],&remaining,&placed)
        }
        // A trailing period still has a 250 ms hold. Keep the cursor
        // visible just after it instead of disappearing during that pause.
        if !placed && len(reveal.text)>0 && strings.HasSuffix(reveal.text,".") &&
            terminalRevealCount(reveal.text,elapsed)==utf8.RuneCountInString(reveal.text) &&
            len(ranges)>0 && rows.End-1>=globalStart && rows.End-1<globalStart+len(visible) {
            last:=rows.End-1-globalStart
            end:=ranges[len(ranges)-1].End
            if end<chatContentWidth(m.width) {
                none:=0
                visible[last]=terminalMaskRow(
                    visible[last],messageBodyRange{Start:end,End:end+1},&none,&placed)
            }
        }
    }
}
