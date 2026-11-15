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

func TestInterferenceBurstsAreShortAndIntermittent(t *testing.T) {
    for _,glitch:=range []bool{false,true} {
        active,quiet,longest:=0,0,0
        consecutive:=0
        for tick:=uint64(0);tick<1200;tick++ {
            _,on:=interferenceBurst(tick,173,glitch)
            if on {active++;consecutive++;if consecutive>longest {longest=consecutive}
            } else {quiet++;consecutive=0}
        }
        if active<16 || quiet<active*4 || longest>17 {
            t.Fatalf("unconvincing interference timing: glitch=%v active=%d quiet=%d longest=%d",glitch,active,quiet,longest)
        }
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
