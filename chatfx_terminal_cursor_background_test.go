package main

import (
    "strings"
    "testing"
)

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
            beforeBG:=interrogationBackgroundAt(lit,10)
            visible,placed:=0,false
            result:=terminalMaskRow(lit,messageBodyRange{Start:10,End:24},
                &visible,&placed,'█',"#D4E9CC",palette.theme)
            if !placed {t.Fatal("typewriter cursor was not painted")}
            if got:=interrogationBackgroundAt(result,11);got!=beforeBG {
                t.Fatalf("cursor leaked theme background into following space: got %v, expected lit %v",got,beforeBG)
            }
            if got:=interrogationBackgroundAt(result,23);got!=beforeBG {
                t.Fatalf("cursor background leaked across unrevealed text: got %v, expected lit %v",got,beforeBG)
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
    beforeBG:=interrogationBackgroundAt(lit,len("Completed!")+1)
    result:=terminalCursorAtEnd(lit,len("Completed!"),'█',"#D4E9CC","#18271E")
    if got:=interrogationBackgroundAt(result,len("Completed!")+1);got!=beforeBG {
        t.Fatalf("persistent cursor left a flat-coloured block: got %v, expected %v",got,beforeBG)
    }
    if !strings.HasPrefix(stripANSISequences(result),"Completed!█") {
        t.Fatal("persistent cursor moved from the end of the message")
    }
}
