package main

import (
    "bytes"
    "encoding/json"
    "reflect"
    "strings"
    "testing"
)

func TestInterferenceCommandsUseExistingRoomAndSelfProtocol(t *testing.T) {
    for _,name:=range []string{"static","glitch"} {
        m:=newMessageInteractionTestModel(100)
        m.room="lobby"
        m.connected=true
        var wire bytes.Buffer
        m.client=&wireClient{enc:json.NewEncoder(&wire)}
        if m.sendMessage("/"+name+" self 20","") {
            t.Fatalf("%s self leaked as chat text",name)
        }
        if wire.Len()!=0 || m.currentChatFX().name!=name {
            t.Fatalf("%s self was not local",name)
        }
        m.chatFXCommand("/stopfx",[]string{"/stopfx","self"})
        if len(m.fxFading)!=0 {t.Fatalf("%s left falling particles",name)}
        if m.sendMessage("/"+name+" 25","") {
            t.Fatalf("%s room command leaked as chat text",name)
        }
        var packet Packet
        if err:=json.NewDecoder(&wire).Decode(&packet);err!=nil {t.Fatal(err)}
        if packet.Type!="fx_start" || packet.FX!=name || packet.FXSeconds!=25 {
            t.Fatalf("%s bad room packet: %+v",name,packet)
        }
        m.receiveChatFX(Packet{Type:"fx_state",FX:name})
        if m.currentChatFX().name!=name {t.Fatalf("%s room state missing",name)}
        m.fxRenderFPS=1
        low:=m.chatFXActivePositions(name,100,30)
        m.fxRenderFPS=60
        if !reflect.DeepEqual(low,m.chatFXActivePositions(name,100,30)) {
            t.Fatalf("%s depends on render FPS",name)
        }
    }
}

func TestMajorInterferenceHasIrregularLongAndShortGaps(t *testing.T) {
    for _,glitch:=range []bool{false,true} {
        shortest,longest:=uint64(2000),uint64(0)
        distinct:=map[uint64]bool{}
        active,quiet,run:=0,0,0
        for event:=uint64(0);event<600;event++ {
            seed:=chatFXHash(173+(event+1)*0x9e3779b97f4a7c15)
            gap:=interferenceGap(seed,glitch)
            if glitch {
                if gap<10 || gap>899 {t.Fatalf("digital gap out of range: %d",gap)}
            } else if gap<3 || gap>1800 {
                t.Fatalf("CRT gap out of range: %d",gap)
            }
            shortest=min(shortest,gap)
            longest=max(longest,gap)
            distinct[gap]=true
        }
        if len(distinct)<60 || shortest>30 || longest<500 {
            t.Fatalf("major timing too predictable: glitch=%v min=%d max=%d distinct=%d",
                glitch,shortest,longest,len(distinct))
        }
        for tick:=uint64(0);tick<1800;tick++ {
            _,on:=interferenceBurst(tick,173,glitch)
            if on {active++;run++;if run>16 {t.Fatal("major burst too long")}
            } else {quiet++;run=0}
        }
        if active==0 || quiet<active*4 {
            t.Fatalf("major disturbances are too persistent: glitch=%v active=%d quiet=%d",glitch,active,quiet)
        }
    }
}

func TestMinorStaticPulsesLastTwoToEightSeconds(t *testing.T) {
    // Measure the continuous envelope, not the occasional star or
    // quantized dot count. Every pulse starts and ends at zero intensity.
    const cycle uint64=330
    shortest,longest:=uint64(330),uint64(0)
    different:=map[uint64]bool{}
    for period:=uint64(0);period<100;period++ {
        salt:=uint64(73)
        start:=period*cycle
        first,last:=uint64(0),uint64(0)
        found:=false
        peak:=0.0
        for tick:=start;tick<start+cycle;tick++ {
            intensity:=minorStaticPulse(tick,salt)
            if intensity<0 || intensity>1 {t.Fatalf("minor static outside 0..1: %f",intensity)}
            if intensity==0 {continue}
            if !found {first=tick;found=true}
            last=tick
            if intensity>peak {peak=intensity}
        }
        if !found {t.Fatal("minor static never appeared")}
        // The two zero endpoints are counted in the designed envelope
        // duration; the nonzero samples occupy duration minus one tick.
        duration:=last-first+2
        if duration<60 || duration>240 {
            t.Fatalf("minor static pulse lasted %d ticks, expected 60..240",duration)
        }
        if peak<0.98 {t.Fatalf("minor static did not fade fully in: %.3f",peak)}
        if minorStaticPulse(start+cycle-1,salt)!=0 {
            t.Fatal("minor static did not leave a quiet interval")
        }
        if duration<shortest {shortest=duration}
        if duration>longest {longest=duration}
        different[duration]=true
    }
    if shortest>90 || longest<210 || len(different)<30 {
        t.Fatalf("minor static durations insufficiently varied: %d..%d ticks, %d distinct",shortest,longest,len(different))
    }
}

func TestMinorGlitchGlyphsIncludePunctuation(t *testing.T) {
    wanted:=[]rune{'.',',','!','^','*','\''}
    available:=make(map[rune]bool,len(minorDigitalGlitchGlyphs))
    for _,glyph:=range minorDigitalGlitchGlyphs {
        if glyph==0 || glyph==' ' {t.Fatalf("unsafe minor glitch glyph: %q",glyph)}
        available[glyph]=true
    }
    for _,glyph:=range wanted {
        if !available[glyph] {t.Fatalf("missing minor glitch character %q",glyph)}
    }
}

func TestMinorCRTInterferenceHasDotFadesAndRareStars(t *testing.T) {
    const width,height=100,30
    m:=newMessageInteractionTestModel(width)
    m.fxSalt=73
    dots,stars,quiet:=0,0,0
    varying:=map[int]bool{}
    for tick:=uint64(0);tick<1800;tick++ {
        m.fxSimSteps=tick
        _,major:=interferenceBurst(tick,m.fxSalt^0x57a71c,false)
        if major {continue}
        rows:=m.staticPositions(width,height)
        n:=0
        for _,row:=range rows {
            for _,cell:=range row {
                switch cell.glyph {
                case '.': dots++;n++
                case '*': stars++;n++
                default: t.Fatalf("minor static emitted major glyph %q",cell.glyph)
                }
            }
        }
        varying[n]=true
        if n==0 {quiet++}
    }
    if dots<100 || stars==0 || quiet==0 || len(varying)<5 {
        t.Fatalf("minor CRT effects not varied: dots=%d stars=%d quiet=%d densities=%d",
            dots,stars,quiet,len(varying))
    }
}

func TestMinorDigitalCorruptionIsColourfulAndSmall(t *testing.T) {
    m:=newMessageInteractionTestModel(100)
    m.fxSalt=73
    colorSet:=map[string]bool{}
    visible,quiet:=0,0
    for tick:=uint64(0);tick<1800;tick++ {
        m.fxSimSteps=tick
        _,textActive:=glitchTextEvent(tick,m.fxSalt)
        if textActive {continue}
        distortedTick:=glitchDistortionTick(tick,m.fxSalt)
        _,major:=interferenceBurst(distortedTick,m.fxSalt^0x6117c4,true)
        if major {continue}
        cells:=0
        for _,row:=range m.glitchPositions(100,30) {
            for _,cell:=range row {
                cells++
                colorSet[cell.color]=true
            }
        }
        if cells>0 {visible++} else {quiet++}
        if cells>50 {t.Fatalf("minor digital fault too large: %d cells",cells)}
    }
    if visible==0 || quiet==0 || len(colorSet)<3 {
        t.Fatalf("minor glitches insufficient: visible=%d quiet=%d colors=%d",
            visible,quiet,len(colorSet))
    }
}

func TestStaticAndGlitchColorsAndCoverage(t *testing.T) {
    for _,name:=range []string{"static","glitch"} {
        m:=newMessageInteractionTestModel(100)
        m.fxRoom=chatFXState{name:name}
        m.fxSalt=73
        colored:=map[string]bool{}
        covered:=0
        quiet:=false
        for tick:=uint64(0);tick<600;tick++ {
            m.fxSimSteps=tick
            rows:=m.chatFXActivePositions(name,100,30)
            if len(rows)!=30 {t.Fatalf("%s produced wrong height",name)}
            n:=0
            for y,row:=range rows {
                for x,cell:=range row {
                    if x<0||x>=100||y<0||y>=30 || cell.color=="" || cell.glyph==' ' || cell.background!="" {
                        t.Fatalf("%s malformed cell at %d,%d",name,x,y)
                    }
                    n++
                    colored[cell.color]=true
                }
            }
            if n==0 {quiet=true}
            covered+=n
        }
        if covered<100 || !quiet {
            t.Fatalf("%s never produced both a burst and quiet frame",name)
        }
        if len(colored)<3 {t.Fatalf("%s has too little color variation: %d",name,len(colored))}
    }
}

func TestInterferenceDoesNotOverwriteChatOrMentionBackground(t *testing.T) {
    for _,name:=range []string{"static","glitch"} {
        m:=newMessageInteractionTestModel(100)
        m.fxRoom=chatFXState{name:name}
        m.fxSalt=73
        prefix:="  [12:00] Alice: Text must stay readable."
        mention:=m.cfg.Theme.MentionBackgroundColor
        found:=false
        for tick:=uint64(0);tick<400;tick++ {
            m.fxSimSteps=tick
            rows:=[]string{
                prefix+strings.Repeat(" ",100-len(prefix)),
                strings.Repeat(" ",100),
                mention+" Mention background "+strings.Repeat(" ",70),
                strings.Repeat(" ",100),
            }
            m.renderChatFXBackground(rows,100)
            if !strings.HasPrefix(stripANSISequences(rows[0]),prefix) {
                t.Fatalf("%s obscured a message",name)
            }
            if !strings.Contains(rows[2],mention) {
                t.Fatalf("%s damaged a mention background",name)
            }
            if rows[1]!=strings.Repeat(" ",100) || rows[3]!=strings.Repeat(" ",100) {found=true}
        }
        if !found {t.Fatalf("%s did not draw in blank rows",name)}
    }
}

func BenchmarkInterferencePositions(b *testing.B) {
    for _,name:=range []string{"static","glitch"} {
        b.Run(name,func(b *testing.B) {
            m:=newMessageInteractionTestModel(160)
            m.fxSalt=73
            b.ReportAllocs()
            for i:=0;i<b.N;i++ {
                m.fxSimSteps=uint64(i)
                if name=="static" {m.staticPositions(160,45)
                } else {m.glitchPositions(160,45)}
            }
        })
    }
}
