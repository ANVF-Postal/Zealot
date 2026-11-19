package main

import (
    "strings"
    "testing"
    "time"
)

// This suite measures the actual visual paths, not just particle positions.
// It excludes SSH transmission and the rest of the Bubble Tea application.
func BenchmarkChatFXRemainingStyledFrame(b *testing.B) {
    sizes:=[]struct{name string;width,height int}{
        {"80x24",80,24},
        {"160x45",160,45},
        {"320x80",320,80},
    }
    for _,effect:=range []string{"aquarium","bubbles","interrogation","terminal"} {
        for _,size:=range sizes {
            b.Run(effect+"/"+size.name,func(b *testing.B) {
                m:=newMessageInteractionTestModel(size.width)
                m.width=size.width
                m.fxRoom=chatFXState{name:effect}
                m.fxSalt=73
                m.fxSimSteps=900
                source:=chatFXBenchmarkRows(size.width,size.height)
                if effect=="aquarium" {
                    m.fxAquariumSalt=73
                    m.fxAquarium=newAquariumState()
                    // Predation and swimmer state use the same update as the
                    // production simulation; leave it stable while rendering.
                    for step:=uint64(1);step<=900;step++ {
                        m.fxAquarium.advance(m,size.width,size.height,float64(step)/30)
                    }
                }
                if effect=="bubbles" {
                    m.fxBubbles=newBubbleState(size.width,size.height,m.fxSalt)
                    m.fxBubbles.advance(900,size.width,size.height)
                }
                if effect=="interrogation" {
                    m.fxInterrogationAmount=0.8
                }
                if effect=="terminal" {
                    m.terminalLocalUntil=terminalForever
                    body:=strings.Repeat("An animated Zealot message with punctuation! ",6)
                    packet:=Packet{
                        Type:"message",ID:"benchmark-terminal",
                        User:User{Name:"Alice",Color:"#92C6D7"},
                        Text:body,Time:time.Now().UTC(),
                    }
                    m.appendMessage(packet)
                    m.terminalTrackMessage(packet)
                    reveal:=m.terminalReveals[0]
                    reveal.started=time.Now().Add(-15*terminalLetterDelay)
                    m.terminalReveals[0]=reveal
                    // Use genuine packet rows and a valid render cache so
                    // the production terminal renderer performs real work.
                    source=m.renderMessageLines()
                    if len(source)>size.height {source=source[:size.height]}
                }
                rows:=make([]string,len(source))
                bytes:=int64(0)
                b.ReportAllocs()
                b.ResetTimer()
                for i:=0;i<b.N;i++ {
                    copy(rows,source)
                    switch effect {
                    case "aquarium","bubbles":
                        m.renderChatFXBackground(rows,size.width)
                    case "interrogation":
                        m.renderInterrogationRows(rows,0)
                        if len(rows)>0 {
                            rows[len(rows)-1]=m.renderInterrogationTypingRow(rows[len(rows)-1],len(rows)-1)
                        }
                    case "terminal":
                        m.renderTerminalRows(rows,0)
                    }
                    for _,row:=range rows {
                        bytes+=int64(len(truncateANSI(row,size.width)))
                    }
                }
                b.StopTimer()
                chatFXStyledFrameSink=int(bytes)
                b.ReportMetric(float64(bytes)/float64(b.N),"ansi-bytes/op")
            })
        }
    }
}

// Aquarium and Bubbles continue simulating at 30 Hz even at a low /fxfps.
// Measure one physics tick separately from drawing a viewport frame.
func BenchmarkChatFXRemainingSimulationTick(b *testing.B) {
    for _,effect:=range []string{"aquarium","bubbles"} {
        for _,size:=range []struct{name string;width,height int}{
            {"80x24",80,24},
            {"160x45",160,45},
            {"320x80",320,80},
        } {
            b.Run(effect+"/"+size.name,func(b *testing.B) {
                m:=newMessageInteractionTestModel(size.width)
                m.fxSalt=73
                m.fxAquariumSalt=73
                aquarium:=newAquariumState()
                bubbles:=newBubbleState(size.width,size.height,73)
                seconds:=30.0
                b.ReportAllocs()
                b.ResetTimer()
                for i:=0;i<b.N;i++ {
                    if effect=="aquarium" {
                        aquarium.advance(m,size.width,size.height,seconds)
                    } else {
                        bubbles.advance(1,size.width,size.height)
                    }
                    seconds+=1.0/30
                }
            })
        }
    }
}
