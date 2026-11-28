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
    terminalClausePause=125*time.Millisecond
    terminalCommaPause=162*time.Millisecond
    terminalCursorBlink=500*time.Millisecond
)

type terminalReveal struct {
    started time.Time
    text string
    duration time.Duration // immutable per message; avoid rescanning punctuation each frame
}

// Zero-duration test fixtures and legacy reveals still take the original path.
func terminalRevealDuration(reveal terminalReveal) time.Duration {
    if reveal.duration>0 {return reveal.duration}
    return terminalTextDuration(reveal.text)
}

// Full stops, question marks and exclamation marks retain the longer
// sentence pause. Commas get a 162 ms beat (200 ms in total), while
// other clause punctuation and paired brackets retain 125 ms.
func terminalPunctuationPause(ch rune) time.Duration {
    switch ch {
    case '.', '?', '!':
        return terminalPeriodPause
    case ',':
        // 38 ms character delay + 162 ms pause = exactly 200 ms.
        return terminalCommaPause
    case ':', ';', '(', ')', '[', ']', '{', '}':
        return terminalClausePause
    default:
        return 0
    }
}

func terminalSentencePause(ch rune) bool {
    return terminalPunctuationPause(ch)==terminalPeriodPause
}

// Blink a single solid block on and off at a fixed rate, independent of
// /fxfps. The invisible phase is one blank cell, not a hollow-square glyph.
func terminalCursorGlyph(now time.Time) rune {
    if (now.UnixNano()/int64(terminalCursorBlink))%2==0 {return '█'}
    return ' '
}

// The cursor occupies an opaque cell in front of background ChatFX.
// Explicitly paint BOTH its foreground and the theme's chat background:
// otherwise a Matrix glyph underneath can leak its colour, or show through
// while the block is blinking off. Never use reverse video or a hollow box.
func terminalCursorPaint(cursor rune,mainTextColor,backgroundColor string) string {
    return terminalCursorPaintWithBackground(cursor,mainTextColor,
        interrogationRGB(backgroundColor,[3]int{17,19,24}))
}

// Paint one opaque cursor cell using the colour already present at that
// position. This is the theme background for ordinary effects, and the
// calculated spotlight shade when Interrogation is running.
func terminalCursorPaintWithBackground(cursor rune,mainTextColor string,bg [3]int) string {
    if cursor==0 {cursor=' '}
    fg:=interrogationRGB(mainTextColor,[3]int{230,230,230})
    return fmt.Sprintf("\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm%c\x1b[39m",
        fg[0],fg[1],fg[2],bg[0],bg[1],bg[2],cursor)
}

// The cursor must be opaque while it is drawn, but its temporary theme
// background must not continue through the rest of the row. In particular,
// Interrogation has already shaded every cell before Terminal paints the
// cursor. Draw the cursor *on that shade*, then restore both colours.
func terminalCursorPaintRestoring(cursor rune, mainTextColor, backgroundColor string, previousFG, previousBG [3]int) string {
    painted:=terminalCursorPaintWithBackground(cursor,mainTextColor,previousBG)
    var out strings.Builder
    out.Grow(len(painted)+48)
    out.WriteString(painted)
    interrogationWriteRGB(&out,48,previousBG)
    interrogationWriteRGB(&out,38,previousFG)
    return out.String()
}

func terminalTextDuration(text string) time.Duration {
    duration:=time.Duration(0)
    for _,ch:=range text {
        duration+=terminalLetterDelay
        duration+=terminalPunctuationPause(ch)
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
        if pause:=terminalPunctuationPause(ch);pause>0 {
            if elapsed<pause {return count}
            elapsed-=pause
        }
    }
    return count
}

func (m model) terminalEnabled() bool {
    if m.fxDisabled || m.fxTerminalMuted || m.cfg.FXBlocked&optionsEffectBit("terminal")!=0 {return false}
    now:=time.Now()
    local:=!m.terminalLocalUntil.IsZero() && now.Before(m.terminalLocalUntil)
    room:=!m.terminalRoomUntil.IsZero() && now.Before(m.terminalRoomUntil)
    return local || room
}

// Cursor wrapping is determined from the fully rendered message, not the
// final rune alone. This handles compact layouts and viewport resizing.
func (m model) terminalCursorNeedsWrap(packet Packet,width int) bool {
    if !m.terminalEnabled() || len(m.terminalReveals)==0 {return false}
    latest:=m.terminalLatestMessage()
    if latest<0 || latest>=len(m.messages) || width<1 {return false}
    tracked:=m.messages[latest]
    if tracked.ID!="" && packet.ID!="" {
        if tracked.ID!=packet.ID {return false}
    } else if tracked.Type!=packet.Type || tracked.Text!=packet.Text ||
        !tracked.Time.Equal(packet.Time) || tracked.User.Name!=packet.User.Name {
        return false
    }
    lines:=m.renderPacketLinesBase(packet,width)
    ranges:=m.terminalBodyRanges(packet,lines)
    for i:=len(ranges)-1;i>=0;i-- {
        if ranges[i].End>ranges[i].Start {
            return ranges[i].End>=chatContentWidth(width)
        }
    }
    return false
}

// When a new message starts typing, the old completed message no longer owns
// a cursor line. Resize only the affected rows; never rewrap the full history.
func (m *model) terminalUpdateCursorRows(previous,latest int) {
    if !m.renderCacheValid || m.width<1 {return}
    for _,i:=range []int{previous,latest} {
        if i<0 || i>=len(m.messages) {continue}
        expected:=m.renderPacketLineCount(m.messages[i],m.width)
        if i<len(m.messageRowRanges) &&
            m.messageRowRanges[i].End-m.messageRowRanges[i].Start!=expected {
            m.resizeCachedMessageRows(i,expected)
        }
    }
    m.clampScroll()
}

// Release a wrapped cursor row when the terminal layer is turned off.
// The message retains every character and its original wrapped height.
func (m *model) terminalClearReveals() {
    previous:=m.terminalLatestMessage()
    clear(m.terminalReveals)
    if previous>=0 {m.terminalUpdateCursorRows(previous,-1)}
}

func (m *model) terminalTrackMessage(packet Packet) {
    if !m.terminalEnabled() {return}
    // Type newly arrived chat, actions and system messages, but not old
    // history, rich interactive artwork, polls, or command/result UI.
    if packet.Type!="message" && packet.Type!="whisper_result" &&
        packet.Type!="system" && packet.Type!="me" {return}
    if packet.XKeyscore>0 || packet.Long || packet.Text=="" {return}
    if m.terminalReveals==nil {m.terminalReveals=make(map[int]terminalReveal)}
    previous:=m.terminalLatestMessage()
    current:=len(m.messages)-1
    m.terminalReveals[current]=terminalReveal{
        // Hard line breaks reserve another row rather than consuming a
        // printable-character tick. The original packet remains untouched.
        started:time.Now(),text:strings.ReplaceAll(strings.ReplaceAll(packet.Text,"\r",""),"\n",""),
    }
    reveal:=m.terminalReveals[current]
    reveal.duration=terminalTextDuration(reveal.text)
    m.terminalReveals[current]=reveal
    m.terminalUpdateCursorRows(previous,current)
}

func (m *model) terminalAdvance(now time.Time) {
    if len(m.terminalReveals)==0 {return}
    if !m.terminalEnabled() {
        m.terminalClearReveals()
        return
    }
    latest:=m.terminalLatestMessage()
    for index,reveal:=range m.terminalReveals {
        if index!=latest && now.Sub(reveal.started)>=terminalRevealDuration(reveal) {
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
func terminalMaskRow(row string,body messageBodyRange,visibleCount *int,cursorPlaced *bool,cursor rune,mainTextColor,backgroundColor string) string {
    if body.End<=body.Start {return row}
    var out strings.Builder
    out.Grow(len(row)+32)
    previousBG:=interrogationRGB(backgroundColor,[3]int{17,19,24})
    previousFG:=interrogationRGB(mainTextColor,[3]int{230,230,230})
    baseBG,baseFG:=previousBG,previousFG
    column:=0
    for i:=0;i<len(row); {
        if row[i]==0x1b {
            end:=ansiSequenceEnd(row,i)
            if end<=i {end=i+1}
            if row[end-1]=='m' && strings.HasPrefix(row[i:end],"\x1b[") {
                params:=row[i+2:end-1]
                previousBG=interrogationANSIBackground(params,previousBG,baseBG)
                previousFG=interrogationANSIColor(params,previousFG,baseFG)
            }
            out.WriteString(row[i:end])
            i=end
            continue
        }
        ch,n:=utf8.DecodeRuneInString(row[i:])
        if n<=0 {break}
        width:=1
        if n>1 {width=lipgloss.Width(string(ch))}
        if width<0 {width=0}
        inside:=column>=body.Start && column<body.End
        if inside {
            if *visibleCount>0 {
                out.WriteString(row[i:i+n])
                *visibleCount--
            } else if !*cursorPlaced && cursor!=0 {
                out.WriteString(terminalCursorPaintRestoring(cursor,mainTextColor,backgroundColor,previousFG,previousBG))
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
func terminalCursorAtEnd(row string,column int,cursor rune,mainTextColor,backgroundColor string) string {
    if column<0 {return row}
    var out strings.Builder
    out.Grow(len(row)+24)
    previousBG:=interrogationRGB(backgroundColor,[3]int{17,19,24})
    previousFG:=interrogationRGB(mainTextColor,[3]int{230,230,230})
    baseBG,baseFG:=previousBG,previousFG
    pos:=0
    painted:=false
    for i:=0;i<len(row); {
        if row[i]==0x1b {
            end:=ansiSequenceEnd(row,i)
            if end<=i {end=i+1}
            if row[end-1]=='m' && strings.HasPrefix(row[i:end],"\x1b[") {
                params:=row[i+2:end-1]
                previousBG=interrogationANSIBackground(params,previousBG,baseBG)
                previousFG=interrogationANSIColor(params,previousFG,baseFG)
            }
            out.WriteString(row[i:end])
            i=end
            continue
        }
        ch,size:=utf8.DecodeRuneInString(row[i:])
        if size<=0 {break}
        width:=1
        if size>1 {width=lipgloss.Width(string(ch))}
        if !painted && pos==column && width>0 {
            out.WriteString(terminalCursorPaintRestoring(cursor,mainTextColor,backgroundColor,previousFG,previousBG))
            // Replace the whole display footprint of any wide Matrix glyph.
            if width>1 {out.WriteString(strings.Repeat(" ",width-1))}
            painted=true
        } else {
            out.WriteString(row[i:i+size])
        }
        pos+=width
        i+=size
    }
    // Defensive fallback for a renderer that provides only ANSI styling
    // (no printable cells) for the cursor-only row. Keep the final letter
    // intact and still draw the block at the requested next-row column.
    if !painted && pos<=column {
        if column>pos {out.WriteString(strings.Repeat(" ",column-pos))}
        out.WriteString(terminalCursorPaintRestoring(cursor,mainTextColor,backgroundColor,previousFG,previousBG))
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
    latest:=m.terminalLatestMessage()
    glyph:=terminalCursorGlyph(now)
    for index,reveal:=range m.terminalReveals {
        if index<0 || index>=len(m.messageRowRanges) || index>=len(m.messages) {continue}
        rows:=m.messageRowRanges[index]
        if rows.Start>=globalStart+len(visible) || rows.End<=globalStart {continue}
        packet:=m.messages[index]
        if packet.Type!="message" && packet.Type!="whisper_result" &&
            packet.Type!="system" && packet.Type!="me" {continue}
        elapsed:=now.Sub(reveal.started)
        duration:=terminalRevealDuration(reveal)
        complete:=elapsed>=duration
        if complete && index!=latest {continue}
        remaining:=terminalRevealCount(reveal.text,elapsed)
        // The full message has already reserved its final rows. Rebuild
        // only its small visible packet here, never the full chat history.
        fullLines:=m.renderPacketLines(packet,m.width)
        ranges:=m.terminalBodyRanges(packet,fullLines)
        placed:=false
        showCursor:=index==latest
        cursor:=rune(0)
        if showCursor {cursor=glyph}
        for row:=rows.Start;row<rows.End;row++ {
            relative:=row-rows.Start
            if relative>=len(ranges) {break}
            if row<globalStart || row>=globalStart+len(visible) {
                if row<globalStart && relative<len(fullLines) {
                    line:=stripANSISequences(fullLines[relative])
                    col:=0
                    for _,r:=range line {
                        width:=lipgloss.Width(string(r))
                        if col>=ranges[relative].Start && col<ranges[relative].End {
                            if remaining>0 {remaining--} else if !placed && showCursor {placed=true}
                        }
                        col+=width
                    }
                }
                continue
            }
            if !complete {
                visible[row-globalStart]=terminalMaskRow(
                    visible[row-globalStart],ranges[relative],&remaining,&placed,cursor,m.cfg.Theme.TextColor,m.cfg.Theme.Background)
            }
        }
        if !showCursor || placed || len(ranges)==0 {continue}
        // With all characters displayed, the cursor stays just after the
        // last body character (including during a trailing punctuation hold).
        // On a completely full row it occupies the final cell instead of
        // exceeding the content width or forcing new wrapping.
        lastRow:=-1
        lastColumn:=-1
        if m.terminalCursorNeedsWrap(packet,m.width) {
            // The final body cell is occupied. The additional row was
            // reserved before rendering, so never replace the last letter.
            lastRow=rows.End-1
            lastColumn=0
        } else {
            for i:=len(ranges)-1;i>=0;i-- {
                if ranges[i].End>ranges[i].Start {
                    lastRow=rows.Start+i
                    lastColumn=ranges[i].End
                    break
                }
            }
        }
        if lastRow<globalStart || lastRow>=globalStart+len(visible) {continue}
        contentWidth:=chatContentWidth(m.width)
        if lastColumn>=contentWidth {continue} // no spare cell: must wrap
        if lastColumn>=0 {
            visible[lastRow-globalStart]=terminalCursorAtEnd(
                visible[lastRow-globalStart],lastColumn,glyph,m.cfg.Theme.TextColor,m.cfg.Theme.Background)
        }
    }
}
