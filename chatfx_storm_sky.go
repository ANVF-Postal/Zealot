package main

import (
    "fmt"
    "math"
    "strings"

    "charm.land/lipgloss/v2"
)

// Detect SGR 7 without allocating a parameter slice or confusing it
// with colour values such as 37, 47, or 107.
func chatFXHasInverseSGR(params string) bool {
    for start:=0;start<len(params); {
        end:=start
        for end<len(params) && params[end]!=';' {end++}
        if end==start+1 && params[start]=='7' {return true}
        start=end+1
    }
    return false
}

// Each weather effect has its own smooth, viewport-relative storm ceiling.
// The smoothstep horizon returns fully to the normal theme background.
func stormSkyBlend(height,y int) float64 {
    return stormSkyBlendForEffect(height,y,"rain")
}

func stormSkyBlendForEffect(height,y int,effect string) float64 {
    if height<=0 || y<0 {return 0}
    coverage:=0.40 // rain
    if effect=="blizzard" {coverage=0.50}
    skyRows:=int(math.Ceil(float64(height)*coverage))
    if y>=skyRows {return 0}
    if skyRows==1 {return 0.88}
    t:=float64(y)/float64(skyRows-1)
    t=t*t*(3-2*t)
    return 0.88*(1-t)
}

// A true darkening operation: on a dark theme the result is darker, never
// more saturated or brighter. This works with custom user themes as well.
func stormSkyColor(base [3]int,weight float64) [3]int {
    if weight<0 {weight=0}
    if weight>1 {weight=1}
    result:=[3]int{}
    for channel:=0;channel<3;channel++ {
        result[channel]=int(math.Round(float64(base[channel])*(1-weight)))
    }
    return result
}

func stormSkyParticleColor(fg [3]int) bool {
    // These are the actual rain and blizzard particle palettes. Ordinary
    // message colours receive only gentle shading to protect legibility.
    switch fg {
    case [3]int{104,156,196}, // rain
         [3]int{246,252,255}, [3]int{201,232,255}, // blizzard
         [3]int{148,203,232}, [3]int{227,244,255}:
        return true
    }
    return false
}

// Darken the entire storm sky, including styled backgrounds, weather
// foregrounds, ordinary chat text, and empty space. Preserve all existing
// SGR attributes and printable characters; the foreground adjustments are
// inserted AFTER source SGR codes so they remain the terminal's final colour.
func stormSkyRow(row,themeBackground,themeForeground string,weight float64,width int) string {
    if weight<=0 || width<=0 {return row}
    normalBG:=interrogationRGB(themeBackground,[3]int{17,19,24})
    normalFG:=interrogationRGB(themeForeground,[3]int{230,230,230})
    currentBG,currentFG:=normalBG,normalFG
    var out strings.Builder
    out.Grow(len(row)+width+128)
    // Never inherit reverse-video or a coloured background from the
    // previous terminal row. Otherwise a shaded droplet foreground can
    // turn into a solid blue bar across the remaining blank cells.
    out.WriteString("\x1b[0m")
    emit:=func() {
        shadedBG:=stormSkyColor(currentBG,weight)
        // Snow and raindrops recede naturally into the upper darkness.
        // Do not make message text difficult to read.
        fgWeight:=0.23*weight
        if stormSkyParticleColor(currentFG) {fgWeight=0.65*weight}
        shadedFG:=stormSkyColor(currentFG,fgWeight)
        fmt.Fprintf(&out,"\x1b[48;2;%d;%d;%dm\x1b[38;2;%d;%d;%dm",
            shadedBG[0],shadedBG[1],shadedBG[2],
            shadedFG[0],shadedFG[1],shadedFG[2])
    }
    emit()
    for i:=0;i<len(row); {
        if row[i]==0x1b {
            end:=ansiSequenceEnd(row,i)
            if end<=i {end=i+1}
            if row[end-1]=='m' && strings.HasPrefix(row[i:end],"\x1b[") {
                params:=row[i+2:end-1]
                currentBG=interrogationANSIBackground(params,currentBG,normalBG)
                currentFG=interrogationANSIColor(params,currentFG,normalFG)
                out.WriteString(row[i:end])
                // Source inverse-video must not swap a shaded droplet's
                // foreground into the background of the rest of the row.
                if chatFXHasInverseSGR(params) {out.WriteString("\x1b[27m")}
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
    if missing:=width-lipgloss.Width(stripANSISequences(row)); missing>0 {
        out.WriteString(strings.Repeat(" ",missing))
    }
    // A full reset ensures the gradient never bleeds into the scrollbar.
    out.WriteString("\x1b[0m")
    return out.String()
}

func (m model) renderStormSkyGradient(rows []string,width int) {
    if m.fxDisabled || len(rows)==0 || width<1 {return}
    switch m.currentChatFX().name {
    case "rain","blizzard":
    default:
        return
    }
    for y:=range rows {
        weight:=stormSkyBlendForEffect(len(rows),y,m.currentChatFX().name)
        if weight<=0 {continue}
        rows[y]=stormSkyRow(rows[y],m.cfg.Theme.Background,m.cfg.Theme.TextColor,weight,width)
    }
}
