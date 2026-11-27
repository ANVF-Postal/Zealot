package main

import (
    "fmt"
    "strings"
    "testing"

    "charm.land/lipgloss/v2"
)

// These benchmarks include deterministic particle placement, the production
// ANSI styling renderer, weather sky colour transforms and the final viewport
// clipping pass. They do not write frames to an actual terminal or SSH socket.
var chatFXStyledFrameSink int

func chatFXBenchmarkRows(width,height int) []string {
    rows:=make([]string,height)
    for y:=range rows {
        // Mix chat text with blank rows: both matter because ChatFX protects
        // message glyphs and styles particles only in the remaining cells.
        body:=""
        if y%3==0 {
            body=fmt.Sprintf("[12:34] Alice: Frame %d - effects behind chat text.",y)
        } else if y%7==0 {
            body="[12:34] Bob: This is a somewhat longer message that occupies part of the viewport."
        }
        if len(body)>width {body=body[:width]}
        color:="\x1b[38;2;230;230;230m"
        if y%3==0 {color="\x1b[38;2;132;234;176m"}
        rows[y]="\x1b[48;2;17;19;24m"+color+body+
            strings.Repeat(" ",width-lipgloss.Width(body))+"\x1b[0m"
    }
    return rows
}

// BenchmarkChatFXStyledFrame is intentionally separate from the position-only
// benchmarks. The 30-second/one-hour cases also expose time-dependent work
// such as replaying static/glitch schedules or past weather phases.
func BenchmarkChatFXStyledFrame(b *testing.B) {
    for _,effect:=range []string{"static","glitch","rain","snow","blizzard","matrix"} {
        for _,size:=range []struct{name string;width,height int}{
            {"80x24",80,24},
            {"160x45",160,45},
            {"320x80",320,80},
        } {
            for _,elapsed:=range []struct{name string;seconds uint64}{
                {"30s",30},
                {"1h",3600},
            } {
                b.Run(effect+"/"+size.name+"/"+elapsed.name,func(b *testing.B) {
                    m:=newMessageInteractionTestModel(size.width)
                    m.fxRoom=chatFXState{name:effect}
                    m.fxSalt=73
                    m.fxSimSteps=elapsed.seconds*30
                    m.fxSimAccumulator=0
                    source:=chatFXBenchmarkRows(size.width,size.height)
                    rows:=make([]string,len(source))
                    outputBytes:=int64(0)
                    b.ReportAllocs()
                    b.ResetTimer()
                    for i:=0;i<b.N;i++ {
                        copy(rows,source)
                        m.renderChatFXBackground(rows,size.width)
                        m.renderStormSkyGradient(rows,size.width)
                        for _,row:=range rows {
                            // Match the production viewport clipping, which
                            // consumes ANSI-rich rows after FX processing.
                            outputBytes+=int64(len(truncateANSI(row,size.width)))
                        }
                    }
                    b.StopTimer()
                    chatFXStyledFrameSink=int(outputBytes)
                    b.ReportMetric(float64(outputBytes)/float64(b.N),"ansi-bytes/op")
                })
            }
        }
    }
}
