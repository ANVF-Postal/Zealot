package main

import (
    "strings"
    "math"
    "testing"
)

func TestStormSkyGradientMatchesWeatherCoverage(t *testing.T) {
    for _,tc:=range []struct {effect string;coverage float64}{
        {"rain",0.40},{"blizzard",0.50},
    } {
        for _,height:=range []int{8,12,20,31,40,60,100} {
            skyRows:=int(math.Ceil(float64(height)*tc.coverage))
            if got:=stormSkyBlendForEffect(height,0,tc.effect);got<0.87 || got>0.89 {
                t.Fatalf("%s height %d: missing dark sky at top: %f",tc.effect,height,got)
            }
            if skyRows>1 && stormSkyBlendForEffect(height,skyRows-1,tc.effect)!=0 {
                t.Fatalf("%s height %d: gradient did not return to theme at horizon",tc.effect,height)
            }
            for y:=skyRows;y<height;y++ {
                if got:=stormSkyBlendForEffect(height,y,tc.effect);got!=0 {
                    t.Fatalf("%s height %d: sky extends below %d rows at row %d",tc.effect,height,skyRows,y)
                }
            }
            previous:=1.0
            for y:=0;y<skyRows;y++ {
                strength:=stormSkyBlendForEffect(height,y,tc.effect)
                if strength>previous {t.Fatalf("%s: dark sky increases with depth at row %d",tc.effect,y)}
                previous=strength
            }
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
            skyRows:=int(math.Ceil(float64(len(rows))*0.40))
            if name=="blizzard" {skyRows=int(math.Ceil(float64(len(rows))*0.50))}
            if rows[skyRows]!=original[skyRows] {
                t.Fatalf("%s sky leaked beyond its horizon",name)
            }
            if rows[skyRows-2]==original[skyRows-2] {
                t.Fatalf("%s sky ended early before its horizon",name)
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

func TestStormSkyDarkensBackgroundAndForeground(t *testing.T) {
    row:="\x1b[48;2;70;80;90m\x1b[38;2;255;160;30mHello "+
        "\x1b[0m\x1b[38;2;104;156;196mrain"
    got:=stormSkyRow(row,"#111318","#E6E6E6",0.88,60)
    if !strings.HasPrefix(stripANSISequences(got),stripANSISequences(row)) {
        t.Fatalf("storm sky changed message: %q",stripANSISequences(got))
    }
    if !strings.Contains(got,"\x1b[38;2;255;160;30m") {
        t.Fatal("source foreground ANSI attributes were lost")
    }
    if background:=interrogationBackgroundAt(got,0); background[0]>=70 || background[1]>=80 || background[2]>=90 {
        t.Fatalf("custom background was not darkened: %v",background)
    }
    if gotWidth:=len(stripANSISequences(got));gotWidth!=60 {
        t.Fatalf("storm sky did not fill the visible row: got %d want 60",gotWidth)
    }
    if unchanged:=stormSkyRow(row,"#111318","#E6E6E6",0,60);unchanged!=row {
        t.Fatal("zero sky tint must preserve the original bytes")
    }
    normal:=[3]int{17,19,24}
    dark:=stormSkyColor(normal,0.88)
    for c:=0;c<3;c++ {
        if dark[c]>=normal[c] {t.Fatalf("dark sky made default theme brighter: %v vs %v",dark,normal)}
    }
    rain:=[3]int{104,156,196}
    if !stormSkyParticleColor(rain) {t.Fatal("rain particle palette not recognized")}
    if !stormSkyParticleColor([3]int{246,252,255}) {t.Fatal("blizzard particle palette not recognized")}
    if stormSkyParticleColor([3]int{255,160,30}) {t.Fatal("chat text incorrectly treated as rain")}
}

func TestStormSkyParticleForegroundIsActuallyDarkened(t *testing.T) {
    for _,code:=range []string{"\x1b[38;2;104;156;196m", "\x1b[38;2;246;252;255m"} {
        row:="\x1b[48;2;17;19;24m"+code+"*"
        shaded:=stormSkyRow(row,"#111318","#E6E6E6",0.88,20)
        if stripANSISequences(shaded)!="*"+strings.Repeat(" ",19) {
            t.Fatalf("weather glyph replaced: %q",stripANSISequences(shaded))
        }
        // A source foreground SGR must be followed by the shaded colour
        // before the weather glyph is drawn.
        source:=strings.Index(shaded,code)
        if source<0 {t.Fatal("source particle colour was lost")}
        glyph:=strings.Index(shaded,"*")
        recolor:=strings.LastIndex(shaded[source+len(code):glyph],"\x1b[38;2;")
        if recolor<0 {t.Fatalf("particle remained full brightness: %q",shaded)}
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
