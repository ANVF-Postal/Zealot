package main

import (
    "strings"
    "testing"
)

func TestStormSkyGradientScalesToQuarterViewport(t *testing.T) {
    for _,height:=range []int{8,12,20,31,40,60,100} {
        skyRows:=(height+3)/4
        if got:=stormSkyBlend(height,0);got<0.8 || got>0.83 {
            t.Fatalf("height %d: missing dark sky at top: %f",height,got)
        }
        if skyRows>1 && stormSkyBlend(height,skyRows-1)!=0 {
            t.Fatalf("height %d: gradient did not reach theme background at horizon",height)
        }
        for y:=skyRows;y<height;y++ {
            if got:=stormSkyBlend(height,y);got!=0 {
                t.Fatalf("height %d: sky extends below top quarter at row %d: %f",height,y,got)
            }
        }
        previous:=1.0
        for y:=0;y<skyRows;y++ {
            strength:=stormSkyBlend(height,y)
            if strength>previous {
                t.Fatalf("height %d: dark sky increases with depth at row %d",height,y)
            }
            previous=strength
        }
    }
}

func TestStormSkyOnlyRainAndBlizzard(t *testing.T) {
    for _,name:=range []string{"rain","blizzard","snow","fire","matrix","interrogation",""} {
        m:=newMessageInteractionTestModel(80)
        m.fxRoom=chatFXState{name:name}
        original:=make([]string,28)
        for y:=range original {original[y]=strings.Repeat(" ",60)}
        rows:=append([]string(nil),original...)
        m.renderStormSkyGradient(rows,60)
        affected:=name=="rain" || name=="blizzard"
        if affected {
            if rows[0]==original[0] || !strings.Contains(rows[0],"\x1b[48;2;") {
                t.Fatalf("%s sky did not shade the top row",name)
            }
            if rows[7]!=original[7] {
                t.Fatalf("%s sky leaked beyond top 25%%",name)
            }
        } else {
            for i:=range rows {
                if rows[i]!=original[i] {t.Fatalf("%s unexpectedly changed row %d",name,i)}
            }
        }
        if affected {
            m.fxDisabled=true
            disabled:=append([]string(nil),original...)
            m.renderStormSkyGradient(disabled,60)
            if disabled[0]!=original[0] {t.Fatalf("%s sky ignored disablefx",name)}
        }
    }
}

func TestStormSkyPreservesTextAndColorsCustomBackground(t *testing.T) {
    row:="\x1b[48;2;70;80;90m\x1b[38;2;255;160;30mHello "+
        "\x1b[0m\x1b[38;2;100;190;255mworld"
    sky:=[3]int{8,19,34}
    got:=stormSkyRow(row,"#111318",sky,0.82,60)
    if !strings.HasPrefix(stripANSISequences(got),stripANSISequences(row)) {
        t.Fatalf("storm sky changed message: %q",stripANSISequences(got))
    }
    if !strings.Contains(got,"\x1b[38;2;255;160;30m") || !strings.Contains(got,"\x1b[38;2;100;190;255m") {
        t.Fatal("storm sky lost message foreground colours")
    }
    if !strings.Contains(got,"\x1b[48;2;70;80;90m") {
        t.Fatal("storm sky removed original background ANSI styling")
    }
    if background:=interrogationBackgroundAt(got,0); background[0]>=70 || background[1]>=80 || background[2]>=90 {
        t.Fatalf("custom background was not tinted: %v",background)
    }
    if gotWidth:=len(stripANSISequences(got));gotWidth!=60 {
        t.Fatalf("storm sky did not fill the visible row: got %d want 60",gotWidth)
    }
    if unchanged:=stormSkyRow(row,"#111318",sky,0,60);unchanged!=row {
        t.Fatal("zero sky tint must preserve the original bytes")
    }
}

func TestStormSkyPreservesParticlesAndMessageText(t *testing.T) {
    for _,name:=range []string{"rain","blizzard"} {
        m:=newMessageInteractionTestModel(80)
        m.fxRoom=chatFXState{name:name}
        m.fxSalt=123
        original:="  [12:01] Alice: The storm is coming"
        rows:=[]string{original,strings.Repeat(" ",70),strings.Repeat(" ",70),strings.Repeat(" ",70),
            strings.Repeat(" ",70),strings.Repeat(" ",70),strings.Repeat(" ",70),strings.Repeat(" ",70)}
        m.renderChatFXBackground(rows,70)
        particlesBefore:=make([]string,len(rows))
        for y,row:=range rows {particlesBefore[y]=stripANSISequences(row)}
        m.renderStormSkyGradient(rows,70)
        for y,row:=range rows {
            if stripANSISequences(row)!=particlesBefore[y] {
                t.Fatalf("%s altered visible particles at row %d",name,y)
            }
        }
        if !strings.HasPrefix(stripANSISequences(rows[0]),original) {
            t.Fatalf("%s sky obscured chat content",name)
        }
    }
}
