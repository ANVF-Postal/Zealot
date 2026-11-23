package main

import (
    "bytes"
    "encoding/json"
    "strings"
    "testing"
    "time"

    "charm.land/lipgloss/v2"
)

func TestTerminalPeriodPause(t *testing.T) {
    text:="Hi. There"
    prefix:=3*terminalLetterDelay
    if got:=terminalRevealCount(text,prefix);got!=3 {
        t.Fatalf("period should be printed: got %d runes",got)
    }
    if got:=terminalRevealCount(text,prefix+terminalPeriodPause-time.Millisecond);got!=3 {
        t.Fatalf("period must pause cursor: got %d runes",got)
    }
    if got:=terminalRevealCount(text,prefix+terminalPeriodPause+terminalLetterDelay);got!=4 {
        t.Fatalf("typing didn't resume after period: got %d runes",got)
    }
    if total:=terminalTextDuration(text);total!=time.Duration(len([]rune(text)))*terminalLetterDelay+terminalPeriodPause {
        t.Fatalf("unexpected total type time: %s",total)
    }
}

func TestTerminalQuestionAndExclamationPause(t *testing.T) {
    for _,punctuation:=range []rune{'.','?','!'} {
        text:="Go"+string(punctuation)+" Next"
        prefix:=3*terminalLetterDelay
        if got:=terminalRevealCount(text,prefix);got!=3 {
            t.Fatalf("%q not printed at correct time: %d",punctuation,got)
        }
        if got:=terminalRevealCount(text,prefix+terminalPeriodPause-time.Millisecond);got!=3 {
            t.Fatalf("%q did not hold cursor for 250 ms: %d",punctuation,got)
        }
        if got:=terminalRevealCount(text,prefix+terminalPeriodPause+terminalLetterDelay);got!=4 {
            t.Fatalf("%q did not resume typing: %d",punctuation,got)
        }
        want:=time.Duration(len([]rune(text)))*terminalLetterDelay+terminalPeriodPause
        if got:=terminalTextDuration(text);got!=want {
            t.Fatalf("%q total typing time %v, expected %v",punctuation,got,want)
        }
    }
}

func TestTerminalClauseAndBracketPauses(t *testing.T) {
    for _,punctuation:=range []rune{',',':',';','(',')','[',']','{','}'} {
        text:="Go"+string(punctuation)+" Next"
        prefix:=3*terminalLetterDelay
        if got:=terminalPunctuationPause(punctuation);got!=terminalClausePause {
            t.Fatalf("%q pause = %v, want %v",punctuation,got,terminalClausePause)
        }
        if got:=terminalRevealCount(text,prefix);got!=3 {
            t.Fatalf("%q should be visible before the pause: got %d runes",punctuation,got)
        }
        if got:=terminalRevealCount(text,prefix+terminalClausePause-time.Millisecond);got!=3 {
            t.Fatalf("%q did not hold cursor for clause pause: got %d runes",punctuation,got)
        }
        if got:=terminalRevealCount(text,prefix+terminalClausePause+terminalLetterDelay);got!=4 {
            t.Fatalf("%q did not resume typing after pause: got %d runes",punctuation,got)
        }
        want:=time.Duration(len([]rune(text)))*terminalLetterDelay+terminalClausePause
        if got:=terminalTextDuration(text);got!=want {
            t.Fatalf("%q total duration = %v, want %v",punctuation,got,want)
        }
    }
}

func TestTerminalMixedPunctuationPausesAccumulate(t *testing.T) {
    text:="value: [one, two]; {done}!"
    clauseCount:=0
    sentenceCount:=0
    for _,ch:=range text {
        switch terminalPunctuationPause(ch) {
        case terminalClausePause: clauseCount++
        case terminalPeriodPause: sentenceCount++
        }
    }
    if clauseCount!=6 || sentenceCount!=1 {
        t.Fatalf("unexpected mixed punctuation counts: %d clauses, %d sentences",clauseCount,sentenceCount)
    }
    expected:=time.Duration(len([]rune(text)))*terminalLetterDelay+
        time.Duration(clauseCount)*terminalClausePause+
        time.Duration(sentenceCount)*terminalPeriodPause
    if got:=terminalTextDuration(text);got!=expected {
        t.Fatalf("mixed punctuation duration = %v, want %v",got,expected)
    }
    if count:=terminalRevealCount(text,expected);count!=len([]rune(text)) {
        t.Fatalf("mixed punctuation did not finish: %d of %d runes",count,len([]rune(text)))
    }
    if terminalPunctuationPause('a')!=0 {
        t.Fatal("normal letters should not pause")
    }
}

func TestTerminalCursorBlinksSolidBlockOnAndOff(t *testing.T) {
    now:=time.Unix(0,0)
    if got:=terminalCursorGlyph(now);got!='█' {t.Fatalf("first cursor phase: %q",got)}
    if got:=terminalCursorGlyph(now.Add(terminalCursorBlink));got!=' ' {
        t.Fatalf("invisible cursor phase: %q",got)
    }
    if got:=terminalCursorGlyph(now.Add(2*terminalCursorBlink));got!='█' {
        t.Fatalf("cursor failed to cycle back: %q",got)
    }
}

func TestTerminalIdleCursorKeepsTickingWithoutFullFrameRate(t *testing.T) {
    m:=newMessageInteractionTestModel(85)
    m.terminalLocalUntil=terminalForever
    packet:=Packet{Type:"message",ID:"idle-blink",Text:"Done!",
        User:User{Name:"Bob",Color:"#94D5AA"},Time:time.Now()}
    m.appendMessage(packet)
    m.terminalTrackMessage(packet)
    if interval:=m.chatFXFrameInterval();interval>time.Second/25 {
        t.Fatalf("typing should use responsive redraws: %s",interval)
    }
    reveal:=m.terminalReveals[0]
    reveal.started=time.Now().Add(-time.Minute)
    m.terminalReveals[0]=reveal
    m.terminalAdvance(time.Now())
    if len(m.terminalReveals)!=1 {
        t.Fatal("idle cursor should stay tracked")
    }
    if interval:=m.chatFXFrameInterval();interval!=terminalCursorBlink {
        t.Fatalf("idle cursor should blink at 2 FPS, got %s",interval)
    }
    if m.ensureChatFXTick()==nil {
        t.Fatal("idle cursor did not schedule its next blink")
    }
}

func TestTerminalCursorUsesThemeForegroundAndOpaqueBackground(t *testing.T) {
    for _,tc:=range []struct{colour,want string}{
        {"#34FF75","\x1b[38;2;52;255;117;48;2;10;20;30m█\x1b[39m"},
        {"#C0C0C0","\x1b[38;2;192;192;192;48;2;10;20;30m█\x1b[39m"},
        {"#FFAA00","\x1b[38;2;255;170;0;48;2;10;20;30m█\x1b[39m"},
    } {
        if got:=terminalCursorPaint('█',tc.colour,"#0A141E");got!=tc.want {
            t.Fatalf("cursor for %s = %q, want %q",tc.colour,got,tc.want)
        }
        if got:=terminalCursorPaint(' ',tc.colour,"#0A141E");got!=strings.Replace(tc.want,"m█","m ",1) {
            t.Fatalf("invisible phase must paint an opaque background cell: %q",got)
        }
    }
    line:="\x1b[48;2;10;20;30m\x1b[38;2;99;80;70mHello       \x1b[0m"
    done:=terminalCursorAtEnd(line,5,'█',"#34FF75","#0A141E")
    if !strings.Contains(done,"\x1b[38;2;52;255;117;48;2;10;20;30m█\x1b[39m") {
        t.Fatal("completed cursor lost theme main text or background colour")
    }
    if strings.Contains(done,"\x1b[7m") || strings.Contains(done,"□") {
        t.Fatal("cursor must never use reverse video or a hollow box")
    }
    off:=terminalCursorAtEnd(line,5,' ',"#34FF75","#0A141E")
    if !strings.Contains(off,"\x1b[38;2;52;255;117;48;2;10;20;30m \x1b[39m") {
        t.Fatal("off phase must erase underlying FX with theme background")
    }
}

func terminalTestEdgeMessage(m model) Packet {
    probe:=Packet{Type:"message",ID:"terminal-border",User:User{Name:"Bob",Color:"#84EAB0"},
        Text:"X",Time:time.Now().UTC()}
    natural:=m.renderPacketLinesBase(probe,m.width)
    ranges:=m.terminalBodyRanges(probe,natural)
    if len(ranges)==0 {return probe}
    capacity:=chatContentWidth(m.width)-ranges[0].Start
    probe.Text=strings.Repeat("X",max(1,capacity))
    return probe
}

func TestTerminalCursorDrawsInANSIOnlyReservedRow(t *testing.T) {
    row:="\x1b[48;2;17;19;24m\x1b[m"
    for _,glyph:=range []rune{'█',' '} {
        got:=terminalCursorAtEnd(row,0,glyph,"#34FF75","#111318")
        want:=terminalCursorPaint(glyph,"#34FF75","#111318")
        if !strings.Contains(got,want) {
            t.Fatalf("ANSI-only row did not receive cursor %q: %q",glyph,got)
        }
        if lipgloss.Width(got)!=1 {
            t.Fatalf("ANSI-only cursor changed width: %d",lipgloss.Width(got))
        }
    }
}

func TestTerminalCursorWrapsBelowFullWidthMessage(t *testing.T) {
    m:=newMessageInteractionTestModel(80)
    m.terminalLocalUntil=terminalForever
    packet:=terminalTestEdgeMessage(m)
    m.appendMessage(packet)
    original:=m.renderPacketLinesBase(packet,m.width)
    if len(original)!=1 || !strings.HasSuffix(strings.TrimSpace(stripANSISequences(original[0])),packet.Text) {
        t.Fatalf("fixture must fill exactly one body row: %q",original)
    }
    m.terminalTrackMessage(packet)
    if !m.terminalCursorNeedsWrap(packet,m.width) {
        t.Fatal("full-width message did not reserve a cursor-only row")
    }
    if rows:=m.messageRowRanges[0];rows.End-rows.Start!=2 {
        t.Fatalf("expected two reserved rows, got %+v",rows)
    }
    reveal:=m.terminalReveals[0]
    reveal.started=time.Now().Add(-time.Minute)
    m.terminalReveals[0]=reveal
    full:=append([]string(nil),m.renderMessageLines()...)
    if len(full)!=2 {t.Fatalf("cached message should have extra row: %d",len(full))}
    visible:=append([]string(nil),full...)
    m.renderTerminalRows(visible,0)
    first:=stripANSISequences(visible[0])
    if !strings.HasSuffix(strings.TrimSpace(first),packet.Text) {
        t.Fatalf("cursor covered the last letter: %q",first)
    }
    if !strings.Contains(visible[1],"\x1b[38;2;") {
        t.Fatalf("cursor not painted on following row: %q",visible[1])
    }
    if len(visible)!=2 {
        t.Fatal("typewriter modified reserved message layout")
    }
}

func TestTerminalWrappedCursorRelayoutOnResizeAndNextMessage(t *testing.T) {
    m:=newMessageInteractionTestModel(80)
    m.terminalLocalUntil=terminalForever
    first:=terminalTestEdgeMessage(m)
    m.appendMessage(first)
    m.terminalTrackMessage(first)
    if got:=len(m.renderMessageLines());got!=2 {
        t.Fatalf("initial edge rows = %d, want 2",got)
    }
    m.width=81
    m.rebuildMessageCache()
    if m.terminalCursorNeedsWrap(first,m.width) {
        t.Fatal("cursor row remained reserved after making terminal wider")
    }
    if got:=len(m.renderMessageLines());got!=1 {
        t.Fatalf("wider layout rows = %d, want 1",got)
    }
    m.width=80
    m.rebuildMessageCache()
    if got:=len(m.renderMessageLines());got!=2 {
        t.Fatalf("restored edge rows = %d, want 2",got)
    }
    second:=Packet{Type:"message",ID:"new-message",User:User{Name:"Alice",Color:"#FFDDAA"},
        Text:"A short message",Time:time.Now().UTC()}
    m.appendMessage(second)
    m.terminalTrackMessage(second)
    if old:=m.messageRowRanges[0];old.End-old.Start!=1 {
        t.Fatalf("old cursor-only row persisted after newer message: %+v",old)
    }
    if got:=len(m.renderMessageLines());got!=2 {
        t.Fatalf("new message must not leave blank cursor row behind: %d",got)
    }
}

func TestTerminalWrappedCursorRowRemovedWhenStopped(t *testing.T) {
    m:=newMessageInteractionTestModel(80)
    m.terminalLocalUntil=terminalForever
    packet:=terminalTestEdgeMessage(m)
    m.appendMessage(packet)
    m.terminalTrackMessage(packet)
    if len(m.renderMessageLines())!=2 {t.Fatal("fixture did not reserve cursor row")}
    m.terminalLocalUntil=time.Time{}
    m.terminalAdvance(time.Now())
    if len(m.terminalReveals)!=0 {t.Fatal("terminal reveal was not cleared")}
    if len(m.renderMessageLines())!=1 {
        t.Fatal("terminal stop left a blank cursor row in chat history")
    }
}

func TestTerminalCursorCoversMatrixGlyphInBothBlinkPhases(t *testing.T) {
    // Simulate a Matrix cell in the blank space directly after chat text.
    matrix:="\x1b[48;2;12;19;25m\x1b[38;2;21;190;70m界\x1b[0m"
    line:="\x1b[48;2;12;19;25mHello"+matrix+"    "
    for _,cursor:=range []rune{'█',' '} {
        got:=terminalCursorAtEnd(line,5,cursor,"#89FF94","#0C1319")
        // The Matrix character must be completely replaced rather than
        // inheriting its green foreground, fading tint or background.
        if strings.Contains(stripANSISequences(got),"界") {
            t.Fatalf("Matrix glyph leaked into cursor cell (%q): %q",cursor,got)
        }
        expected:=terminalCursorPaint(cursor,"#89FF94","#0C1319")
        if !strings.Contains(got,expected) {
            t.Fatalf("Matrix overlapped cursor (%q): %q",cursor,got)
        }
    }
}


func TestTerminalCursorStaysOnLatestCompletedMessage(t *testing.T) {
    m:=newMessageInteractionTestModel(85)
    m.terminalLocalUntil=terminalForever
    for _,body:=range []string{"First message?","Second message!"} {
        p:=Packet{Type:"message",ID:body,User:User{Name:"Alice",Color:"#92C6D7"},
            Text:body,Time:time.Now()}
        m.appendMessage(p)
        m.terminalTrackMessage(p)
    }
    old:=time.Now().Add(-10*time.Second)
    for key,reveal:=range m.terminalReveals {
        reveal.started=old
        m.terminalReveals[key]=reveal
    }
    m.terminalAdvance(time.Now())
    if len(m.terminalReveals)!=1 || m.terminalLatestMessage()!=1 {
        t.Fatalf("only newest message should retain cursor: %+v",m.terminalReveals)
    }
    rows:=append([]string(nil),m.renderMessageLines()...)
    m.renderTerminalRows(rows,0)
    text:=strings.Join(rows,"\n")
    // Both blink phases are valid at wall-clock time. Verify that the
    // text itself stays intact and the earlier message remains cursor-free.
    parts:=strings.Split(text,"\n")
    if len(parts)<2 || strings.Contains(parts[0],"█") || strings.Contains(parts[0],"□") {
        t.Fatalf("cursor must not remain on the previous message: %q",text)
    }
    if !strings.Contains(stripANSISequences(text),"First message?") ||
        !strings.Contains(stripANSISequences(text),"Second message!") {
        t.Fatal("completed text was changed by persistent cursor")
    }
    m.terminalLocalUntil=time.Time{}
    m.terminalAdvance(time.Now())
    if len(m.terminalReveals)!=0 {t.Fatal("cursor should disappear after terminal stops")}
}

func TestTerminalRowsReserveWrappedMessageAndKeepColours(t *testing.T) {
    m:=newMessageInteractionTestModel(85)
    m.width=85
    m.terminalLocalUntil=terminalForever
    msg:=Packet{
        Type:"message",ID:"terminal-wrapped",User:User{Name:"Alice",Color:"#92C6D7"},
        Text:"A multi-line message with plenty of characters. Then a second statement that should wrap without causing the chat pane to jump.",
        Time:time.Now().UTC(),
    }
    m.appendMessage(msg)
    m.terminalTrackMessage(msg)
    if len(m.terminalReveals)!=1 {t.Fatal("received message was not tracked")}
    full:=append([]string(nil),m.renderMessageLines()...)
    if len(full)<2 {t.Fatalf("message didn't wrap: %d rows",len(full))}
    hidden:=append([]string(nil),full...)
    m.renderTerminalRows(hidden,0)
    if len(hidden)!=len(full) {t.Fatal("typewriter changed reserved message height")}
    rendered:=strings.Join(hidden,"\n")
    if strings.Contains(stripANSISequences(rendered),"plenty of characters") {
        t.Fatal("hidden body leaked before typing")
    }
    if !strings.Contains(rendered,"\x1b[") {
        t.Fatal("missing ANSI styling")
    }
    if !strings.Contains(rendered,"92;") && !strings.Contains(rendered,"92C6D7") {
        // Most ANSI renderers convert colours to 24-bit decimal escapes.
        t.Log("colour preserved in original row format; cursor didn't strip ANSI")
    }
    if !strings.Contains(rendered,"\x1b[") {
        t.Fatal("the underlying ANSI styling was stripped")
    }
    state:=m.terminalReveals[0]
    state.started=time.Now().Add(-15*terminalLetterDelay)
    m.terminalReveals[0]=state
    partial:=append([]string(nil),full...)
    m.renderTerminalRows(partial,0)
    if !strings.Contains(strings.Join(partial,"\n"),"multi-line") {
        t.Fatal("the first part of the message wasn't revealed")
    }
    if strings.Contains(stripANSISequences(strings.Join(partial,"\n")),"second statement") {
        t.Fatal("later wrapped text appeared prematurely")
    }
    state.started=time.Now().Add(-terminalTextDuration(msg.Text)-time.Second)
    m.terminalReveals[0]=state
    complete:=append([]string(nil),full...)
    m.renderTerminalRows(complete,0)
    if !m.terminalEnabled() || m.terminalLatestMessage()!=0 {
        t.Fatal("newest completed message must retain its blinking cursor")
    }
    if len(complete)!=len(full) {t.Fatal("completed cursor shifted reserved rows")}
    m.terminalAdvance(time.Now())
    if len(m.terminalReveals)!=1 {
        t.Fatal("finished newest message lost its persistent blinking cursor")
    }
}

func TestTerminalExplicitMultilinePreservesReservedRows(t *testing.T) {
    m:=newMessageInteractionTestModel(65)
    m.terminalLocalUntil=terminalForever
    packet:=Packet{
        Type:"message",ID:"multiple-lines",
        User:User{Name:"Bob",Color:"#94D5AA"},
        Text:"First line.\nSecond line is visible later.\nThird line waits.",
        Time:time.Now(),
    }
    m.appendMessage(packet)
    m.terminalTrackMessage(packet)
    original:=append([]string(nil),m.renderMessageLines()...)
    if len(original)<3 {t.Fatalf("expected reserved explicit lines, got %d",len(original))}
    if got:=m.terminalReveals[0].text;strings.Contains(got,"\n") {
        t.Fatalf("newline incorrectly consumes a typewriter glyph: %q",got)
    }
    state:=m.terminalReveals[0]
    state.started=time.Now().Add(-time.Duration(len([]rune("First line.")))*terminalLetterDelay-
        terminalPeriodPause-2*terminalLetterDelay)
    m.terminalReveals[0]=state
    rows:=append([]string(nil),original...)
    m.renderTerminalRows(rows,0)
    if len(rows)!=len(original) {t.Fatal("multiline typewriter shifted chat layout")}
    joined:=stripANSISequences(strings.Join(rows,"\n"))
    if !strings.Contains(joined,"First line.") {t.Fatal("completed first line disappeared")}
    if strings.Contains(joined,"Second line is visible later.") ||
        strings.Contains(joined,"Third line waits.") {
        t.Fatal("future multiline text was revealed prematurely")
    }
}

func TestTerminalCursorStaysAfterFinalPeriod(t *testing.T) {
    m:=newMessageInteractionTestModel(85)
    m.terminalLocalUntil=terminalForever
    p:=Packet{Type:"message",ID:"period-hold",
        User:User{Name:"Alice",Color:"#92C6D7"},
        Text:"Hello.",Time:time.Now()}
    m.appendMessage(p)
    m.terminalTrackMessage(p)
    state:=m.terminalReveals[0]
    state.started=time.Now().Add(-time.Duration(len([]rune(p.Text)))*terminalLetterDelay-100*time.Millisecond)
    m.terminalReveals[0]=state
    rows:=append([]string(nil),m.renderMessageLines()...)
    m.renderTerminalRows(rows,0)
    if !m.terminalEnabled() || m.terminalLatestMessage()!=0 {
        t.Fatal("cursor lost its position during final punctuation pause")
    }
}

func TestTerminalControlsRemainIndependentOfBackgroundFX(t *testing.T) {
    m:=newMessageInteractionTestModel(100)
    m.room="lobby"
    m.connected=true
    var outgoing bytes.Buffer
    m.client=&wireClient{enc:json.NewEncoder(&outgoing)}
    if !isChatFXCommand("/terminal") {t.Fatal("/terminal unregistered")}
    m.chatFXCommand("/terminal",[]string{"/terminal"})
    var p Packet
    if err:=json.NewDecoder(&outgoing).Decode(&p);err!=nil {t.Fatal(err)}
    if p.Type!="fx_terminal_start" || p.FXSeconds!=0 {
        t.Fatalf("wrong terminal room packet: %+v",p)
    }
    m.receiveChatFX(Packet{Type:"fx_terminal_state",FXUntil:terminalForever})
    m.receiveChatFX(Packet{Type:"fx_state",FX:"rain"})
    if !m.terminalEnabled() || m.currentChatFX().name!="rain" {
        t.Fatal("rain displaced the terminal layer")
    }
    m.receiveChatFX(Packet{Type:"fx_state",FX:"aquarium"})
    if !m.terminalEnabled() || m.currentChatFX().name!="aquarium" {
        t.Fatal("aquarium displaced the terminal layer")
    }
    m.chatFXCommand("/terminal",[]string{"/terminal","self","10"})
    if !m.terminalEnabled() || m.terminalLocalUntil.IsZero() {
        t.Fatal("local terminal didn't stack")
    }
    m.receiveChatFX(Packet{Type:"fx_terminal_state"})
    if !m.terminalEnabled() {t.Fatal("room terminal stop incorrectly stopped local terminal")}
    m.chatFXCommand("/terminal",[]string{"/terminal","self","off"})
    if m.terminalEnabled() {t.Fatal("self off did not stop terminal")}
    if m.currentChatFX().name!="aquarium" {t.Fatal("terminal off stopped the aquarium")}
}

func TestTerminalNeverAnimatesReplayedOrControlMessages(t *testing.T) {
    m:=newMessageInteractionTestModel(90)
    message:=Packet{Type:"message",ID:"history",Text:"old message",User:User{Name:"Bob"},Time:time.Now()}
    m.appendMessages([]Packet{message})
    if len(m.terminalReveals)!=0 {t.Fatal("history replay was tracked as new text")}
    m.terminalLocalUntil=terminalForever
    m.terminalTrackMessage(message)
    if len(m.terminalReveals)!=1 {t.Fatal("new message not tracked")}
    notice:=Packet{Type:"system",Text:"Room joined",Time:time.Now()}
    m.appendMessage(notice)
    m.terminalTrackMessage(notice)
    if len(m.terminalReveals)!=2 {t.Fatal("incoming system notice should be typed too")}
    m.fxDisabled=true
    if m.terminalEnabled() {t.Fatal("disablefx didn't suppress terminal")}
}

func TestTerminalSystemAndActionRowsKeepTheirHeaders(t *testing.T) {
    m:=newMessageInteractionTestModel(90)
    m.width=90
    m.terminalLocalUntil=terminalForever
    packets:=[]Packet{
        {Type:"system",ID:"terminal-system",Text:"Something interesting happened.",Time:time.Now()},
        {Type:"me",ID:"terminal-action",Text:"waves to everybody.",User:User{Name:"Alice",Color:"#AABBCC"},Time:time.Now()},
    }
    for _,packet:=range packets {
        m.appendMessage(packet)
        m.terminalTrackMessage(packet)
    }
    if len(m.terminalReveals)!=len(packets) {t.Fatal("system or action message was not tracked")}
    original:=append([]string(nil),m.renderMessageLines()...)
    visible:=append([]string(nil),original...)
    m.renderTerminalRows(visible,0)
    if len(visible)!=len(original) {t.Fatal("terminal altered row allocation")}
    joined:=stripANSISequences(strings.Join(visible,"\n"))
    if !strings.Contains(joined,"Alice") || !strings.Contains(joined,"[") {
        t.Fatal("terminal hid message metadata while typing")
    }
    if strings.Contains(joined,"interesting happened") || strings.Contains(joined,"waves to everybody") {
        t.Fatal("untyped system/action body was visible")
    }
}

func TestTerminalMaskPreservesWidthAndUsernames(t *testing.T) {
    line:="\x1b[48;2;17;19;24m\x1b[38;2;55;200;245m[12:00] Alice: Hello world\x1b[0m"
    body:=messageBodyRange{Start:15,End:26}
    remaining:=0
    placed:=false
    result:=terminalMaskRow(line,body,&remaining,&placed,'█',"#34FF75","#111318")
    plain:=stripANSISequences(result)
    if !strings.Contains(plain,"Alice:") {t.Fatal("terminal erased the author")}
    if strings.Contains(plain,"world") {t.Fatal("terminal revealed unreached text")}
    if !strings.Contains(result,"\x1b[38;2;52;255;117;48;2;17;19;24m█\x1b[39m") {t.Fatalf("theme-colored opaque cursor absent: %q",result)}
    if strings.Contains(result,"\x1b[7m") || !strings.Contains(result,"\x1b[48;2;17;19;24m") {
        t.Fatal("cursor inverted or stripped message background")
    }
}
