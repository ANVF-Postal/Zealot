package main

import (
    "strings"
    "testing"
    "unicode/utf8"
)

// The older background-at test helper indexes UTF-8 bytes, not terminal
// columns. This test includes a three-byte, one-cell block cursor, so count
// printable runes to compare backgrounds at the correct display columns.
func terminalTestBackgroundAtColumn(line string, column int, theme string) [3]int {
    normal:=interrogationRGB(theme,[3]int{17,19,24})
    bg:=normal
    x:=0
    for i:=0;i<len(line); {
        if line[i]==0x1b {
            end:=ansiSequenceEnd(line,i)
            if end<=i {end=i+1}
            if line[end-1]=='m' && strings.HasPrefix(line[i:end],"\x1b[") {
                bg=interrogationANSIBackground(line[i+2:end-1],bg,normal)
            }
            i=end
            continue
        }
        if x==column {return bg}
        _,n:=utf8.DecodeRuneInString(line[i:])
        if n<=0 {break}
        i+=n
        x++
    }
    return bg
}

// Interrogation shades message backgrounds before Terminal adds its block
// cursor. The cursor should cover only one cell, never paint a same-colour
// rectangle through the remaining unrevealed message.
func TestTerminalTypingCursorRestoresInterrogationBackground(t *testing.T) {
    for _,palette:=range []struct{name,theme string}{
        {"woodland","#18271E"},{"default","#111318"},
    } {
        t.Run(palette.name,func(t *testing.T) {
            const width=36
            original:=strings.Repeat(" ",width)
            lit:=interrogationLightRow(original,palette.theme,"#D4E9CC",width,24,19,0.88,0,1)
            visible,placed:=0,false
            result:=terminalMaskRow(lit,messageBodyRange{Start:10,End:24},
                &visible,&placed,'█',"#D4E9CC",palette.theme)
            if !placed {t.Fatal("typewriter cursor was not painted")}
            if got:=terminalTestBackgroundAtColumn(result,11,palette.theme);got!=terminalTestBackgroundAtColumn(lit,11,palette.theme) {
                t.Fatalf("cursor leaked theme background into following space: got %v, expected lit %v",got,terminalTestBackgroundAtColumn(lit,11,palette.theme))
            }
            if got:=terminalTestBackgroundAtColumn(result,23,palette.theme);got!=terminalTestBackgroundAtColumn(lit,23,palette.theme) {
                t.Fatalf("cursor background leaked across unrevealed text: got %v, expected lit %v",got,terminalTestBackgroundAtColumn(lit,23,palette.theme))
            }
            if stripANSISequences(result)!=original[:10]+"█"+original[11:] {
                t.Fatal("cursor changed printable content or row width")
            }
        })
    }
}

func TestTerminalPersistentCursorRestoresInterrogationBackground(t *testing.T) {
    const width=40
    original:="Completed!"+strings.Repeat(" ",width-len("Completed!"))
    lit:=interrogationLightRow(original,"#18271E","#D4E9CC",width,24,20,0.85,0,1)
    beforeBG:=terminalTestBackgroundAtColumn(lit,len("Completed!")+1,"#18271E")
    result:=terminalCursorAtEnd(lit,len("Completed!"),'█',"#D4E9CC","#18271E")
    if got:=terminalTestBackgroundAtColumn(result,len("Completed!")+1,"#18271E");got!=beforeBG {
        t.Fatalf("persistent cursor left a flat-coloured block: got %v, expected %v",got,beforeBG)
    }
    if !strings.HasPrefix(stripANSISequences(result),"Completed!█") {
        t.Fatal("persistent cursor moved from the end of the message")
    }
}
