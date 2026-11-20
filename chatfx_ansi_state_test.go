package main

import (
    "strings"
    "testing"
)

// ANSI effects are row-wide colour transforms. Any incoming reverse-video
// attribute must be cleared before printable characters are emitted: with
// reverse active a blue raindrop's foreground becomes a blue horizontal
// *background* streak, and Matrix's colour palette becomes rainbow bands.
func assertChatFXNoReverseAtPrintable(t *testing.T, row string) {
    t.Helper()
    reversed:=false
    for i:=0;i<len(row); {
        if row[i]==0x1b {
            end:=ansiSequenceEnd(row,i)
            if end<=i {t.Fatalf("invalid ANSI sequence at %d",i)}
            if strings.HasPrefix(row[i:end],"\x1b[") && row[end-1]=='m' {
                for _,field:=range strings.Split(row[i+2:end-1],";") {
                    switch field {
                    case "", "0": reversed=false
                    case "7": reversed=true
                    case "27": reversed=false
                    }
                }
            }
            i=end
            continue
        }
        if reversed {
            t.Fatalf("reverse-video ANSI state reached printable cell at byte %d: %q",i,row)
        }
        i++
    }
}

func TestRainGradientCannotInvertParticleForegroundIntoBackground(t *testing.T) {
    const width=80
    // Deliberately leave inverse-video active in the source, including
    // across a droplet and several blank cells.
    raw:="\x1b[7m\x1b[38;2;104;156;196m."+strings.Repeat(" ",width-1)
    shaded:=stormSkyRow(raw,"#111318","#E6E6E6",0.70,width)
    if stripANSISequences(shaded)!="."+strings.Repeat(" ",width-1) {
        t.Fatal("storm gradient altered the visible weather cells")
    }
    assertChatFXNoReverseAtPrintable(t,shaded)
    for _,column:=range []int{0,1,20,width-1} {
        if got:=interrogationBackgroundAt(shaded,column);got!=stormSkyColor([3]int{17,19,24},0.70) {
            t.Fatalf("rain column %d leaked a non-theme background: %v",column,got)
        }
    }
}

func TestInterrogationCannotInvertPreviousMatrixColoursIntoBackground(t *testing.T) {
    const width=80
    raw:="\x1b[7m\x1b[38;2;36;218;99mM"+strings.Repeat(" ",width-1)
    shaded:=interrogationLightRow(raw,"#111318","#E6E6E6",width,24,4,0.8,0,1)
    if stripANSISequences(shaded)!="M"+strings.Repeat(" ",width-1) {
        t.Fatal("interrogation lighting altered the underlying Matrix glyphs")
    }
    assertChatFXNoReverseAtPrintable(t,shaded)
    for _,column:=range []int{0,1,20,width-1} {
        bg:=interrogationBackgroundAt(shaded,column)
        for channel:=0;channel<3;channel++ {
            if bg[channel]<0 || bg[channel]>24 {
                t.Fatalf("interrogation column %d inherited colour background %v",column,bg)
            }
        }
    }
}

func TestActualRainAndMatrixFramesHaveNoBrightBackgroundStreaks(t *testing.T) {
    const width,height=90,28
    for _,effect:=range []string{"rain","matrix"} {
        m:=newMessageInteractionTestModel(width)
        m.fxRoom=chatFXState{name:effect}
        m.fxSalt=73
        m.fxSimSteps=900
        rows:=chatFXBenchmarkRows(width,height)
        m.renderChatFXBackground(rows,width)
        if effect=="rain" {
            m.renderStormSkyGradient(rows,width)
        } else {
            m.fxRoom=chatFXState{name:"interrogation"}
            m.fxInterrogationAmount=0.90
            m.renderInterrogationRows(rows,0)
        }
        for y,row:=range rows {
            assertChatFXNoReverseAtPrintable(t,row)
            for x:=0;x<width;x++ {
                bg:=interrogationBackgroundAt(row,x)
                if bg[0]<0 || bg[1]<0 || bg[2]<0 {
                    t.Fatalf("%s row %d col %d left background undefined",effect,y,x)
                }
                if bg[0]>70 || bg[1]>70 || bg[2]>70 {
                    t.Fatalf("%s row %d col %d has unexpectedly bright background %v",effect,y,x,bg)
                }
            }
        }
    }
}
