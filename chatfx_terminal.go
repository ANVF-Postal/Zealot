package main

import (
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
)

type terminalReveal struct {
    started time.Time
    text string
}

func terminalTextDuration(text string) time.Duration {
    duration:=time.Duration(0)
    for _,ch:=range text {
        duration+=terminalLetterDelay
        if ch=='.' {duration+=terminalPeriodPause}
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
        if ch=='.' {
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
    // Message bodies are typed, not system control notices, old history,
    // outgoing echoes, or special interactive artwork/polls.
    if packet.Type!="message" && packet.Type!="whisper_result" {return}
    if packet.XKeyscore>0 || packet.Text=="" {return}
    if m.terminalReveals==nil {m.terminalReveals=make(map[int]terminalReveal)}
    m.terminalReveals[len(m.messages)-1]=terminalReveal{
        started:time.Now(),text:packet.Text,
    }
}

func (m *model) terminalAdvance(now time.Time) {
    if len(m.terminalReveals)==0 {return}
    if !m.terminalEnabled() {
        clear(m.terminalReveals)
        return
    }
    for index,reveal:=range m.terminalReveals {
        if now.Sub(reveal.started)>=terminalTextDuration(reveal.text) {
            delete(m.terminalReveals,index)
        }
    }
}

// Paint the *already wrapped* full message, hiding only body glyphs that
// haven't been typed yet. This preserves stable row counts, original
// colours, hover/reply geometry, ANSI escapes and the scrollbar position.
// visibleCount is shared across all rows of one message.
func terminalMaskRow(row string,body messageBodyRange,visibleCount *int,cursorPlaced *bool) string {
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
            } else if !*cursorPlaced {
                // Inverse video preserves the row's existing foreground
                // and background colours; resetting SGR here would leak
                // black patches into the theme's painted chat area.
                out.WriteString("\x1b[7m█\x1b[27m")
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
        if packet.Type!="message" && packet.Type!="whisper_result" {continue}
        elapsed:=now.Sub(reveal.started)
        if elapsed>=terminalTextDuration(reveal.text) {continue}
        remaining:=terminalRevealCount(reveal.text,elapsed)
        ranges:=m.messageBodyRangesForLineCount(packet,m.width,rows.End-rows.Start)
        placed:=false
        // Consume all prior rows, even when scrolled halfway through a
        // long message. The reveal remains continuous across wrapping.
        for row:=rows.Start;row<rows.End;row++ {
            relative:=row-rows.Start
            if relative>=len(ranges) {break}
            if row<globalStart || row>=globalStart+len(visible) {
                // Count real displayed body runes in omitted previous rows.
                if row<globalStart && row>=0 && row<len(m.renderedLines) {
                    plain:=[]rune(stripANSISequences(m.renderedLines[row]))
                    for col,r:=range plain {
                        if col>=ranges[relative].Start && col<ranges[relative].End && r!=0 {
                            if remaining>0 {remaining--} else if !placed {placed=true}
                        }
                    }
                }
                continue
            }
            visible[row-globalStart]=terminalMaskRow(
                visible[row-globalStart],ranges[relative],&remaining,&placed)
        }
    }
}
