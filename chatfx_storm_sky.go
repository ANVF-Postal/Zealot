package main

import (
    "fmt"
    "math"
    "strings"

    "charm.land/lipgloss/v2"
)

// Storm weather gently darkens the upper quarter of the chat viewport.
// Using the visible height, rather than the amount of stored history, keeps
// the horizon fixed when users scroll or resize the terminal.
func stormSkyBlend(height,y int) float64 {
    if height<=0 || y<0 {return 0}
    skyRows:=int(math.Ceil(float64(height)*0.25))
    if y>=skyRows {return 0}
    if skyRows==1 {return 0.82}
    t:=float64(y)/float64(skyRows-1)
    t=t*t*(3-2*t) // smoothstep, with zero tint at the horizon
    return 0.82*(1-t)
}

func stormSkyColor(base,sky [3]int,weight float64) [3]int {
    result:=[3]int{}
    for channel:=0;channel<3;channel++ {
        result[channel]=int(math.Round(float64(base[channel])*(1-weight)+float64(sky[channel])*weight))
    }
    return result
}

// Apply the sky to actual background cells, including explicit message
// backgrounds and blank space, without changing glyphs or foreground colors.
// Each ANSI reset or background selection gets the appropriate sky tint.
func stormSkyRow(row string,themeBackground string,sky [3]int,weight float64,width int) string {
    if weight<=0 || width<=0 {return row}
    normal:=interrogationRGB(themeBackground,[3]int{17,19,24})
    current:=normal
    var out strings.Builder
    out.Grow(len(row)+width+80)
    emit:=func() {
        color:=stormSkyColor(current,sky,weight)
        fmt.Fprintf(&out,"\x1b[48;2;%d;%d;%dm",color[0],color[1],color[2])
    }
    emit()
    for i:=0;i<len(row); {
        if row[i]==0x1b {
            end:=ansiSequenceEnd(row,i)
            if end<=i {end=i+1}
            if row[end-1]=='m' && strings.HasPrefix(row[i:end],"\x1b[") {
                current=interrogationANSIBackground(row[i+2:end-1],current,normal)
                out.WriteString(row[i:end])
                emit()
            } else {
                out.WriteString(row[i:end])
            }
            i=end
            continue
        }
        out.WriteByte(row[i])
        i++
    }
    // Backgrounds must extend across every cell even if the original row
    // was short or entirely empty. Never add printable non-space characters.
    if missing:=width-lipgloss.Width(stripANSISequences(row)); missing>0 {
        out.WriteString(strings.Repeat(" ",missing))
    }
    out.WriteString("\x1b[0m")
    return out.String()
}

func (m model) renderStormSkyGradient(rows []string,width int) {
    if m.fxDisabled || len(rows)==0 || width<1 {return}
    name:=m.currentChatFX().name
    var sky [3]int
    switch name {
    case "rain":
        sky=[3]int{8,19,34} // deep rainy midnight blue
    case "blizzard":
        sky=[3]int{13,27,41} // cold storm-blue
    default:
        return
    }
    for y:=range rows {
        strength:=stormSkyBlend(len(rows),y)
        if strength<=0 {continue}
        rows[y]=stormSkyRow(rows[y],m.cfg.Theme.Background,sky,strength,width)
    }
}
