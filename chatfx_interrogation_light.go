package main

import (
    "fmt"
    "math"
    "strings"
    "unicode/utf8"

    "charm.land/lipgloss/v2"
)

// The lamp is just above the bottom of the message pane, not the input.
// An elliptical Gaussian produces a rounded vignette in terminal cells.
// The floor retains 25% brightness so earlier messages remain decipherable.
func interrogationLightAt(x,y,width,height int, emphasis,flicker float64) float64 {
    if width<1 || height<1 {return 1}
    cx:=float64(width-1)*0.50
    cy:=float64(height-1)-math.Min(1.5,float64(height-1)*0.15)
    rx:=math.Max(3,float64(width)*0.55)
    ry:=math.Max(3,math.Min(11,float64(height)*0.42))
    dx:=(float64(x)-cx)/rx
    dy:=(float64(y)-cy)/ry
    glow:=math.Exp(-1.55*(dx*dx+dy*dy))
    // A strong, but never entirely white, lower-centre light pool.
    light:=0.25+0.68*glow*flicker
    // Conversation emphasis is part of the light, not immunity from it.
    light+=0.18*emphasis*glow
    return math.Max(0.23,math.Min(1,light))
}

// Very rare, short fluorescent dips, deterministic at the 30 Hz simulation
// rate. No randomly moving light position, no flashing white frames.
func interrogationLampIntensity(tick,salt uint64) float64 {
    const cycle uint64=390 // roughly 13 seconds
    block:=tick/cycle
    seed:=chatFXHash(salt^0x1a731a7e+block*0x9e3779b97f4a7c15)
    if seed%3==0 {return 1} // some periods stay completely steady
    begin:=uint64(60+(seed>>9)%250)
    age:=tick%cycle
    length:=uint64(3+(seed>>18)%5)
    if age<begin || age>=begin+length {return 1}
    // Smooth, mild dip followed by recovery.
    phase:=float64(age-begin)/float64(length)
    return 1-(0.06+0.06*chatFXUnit(seed+9))*math.Sin(math.Pi*phase)
}

// Keep brightness changes per-column without re-rendering messages or touching
// their cached contents. SGR backgrounds, bold/underline, and ANSI hitbox
// offsets are retained. Colour runs are quantized to avoid excess ANSI output.
func interrogationLightRow(row,background,foreground string,width,height,y int,amount,emphasis,flicker float64) string {
    if amount<=0 || width<1 {return row}
    bg:=interrogationRGB(background,[3]int{17,19,24})
    normal:=interrogationRGB(foreground,[3]int{230,230,230})
    current:=normal
    var output strings.Builder
    output.Grow(len(row)+width*14)
    x:=0
    lastColor:=[3]int{-1,-1,-1}
    emit:=func() {
        sample:=interrogationLightAt(min(width-1,x),y,width,height,emphasis,flicker)
        // At zero effect strength every pixel matches the source.
        opacity:=1-amount*(1-sample)
        // Quantize solely for output compactness, not for animation time.
        opacity=math.Round(opacity*48)/48
        blend:=[3]int{}
        for c:=0;c<3;c++ {
            blend[c]=int(math.Round(float64(bg[c])*(1-opacity)+float64(current[c])*opacity))
        }
        if blend!=lastColor {
            fmt.Fprintf(&output,"\x1b[38;2;%d;%d;%dm",blend[0],blend[1],blend[2])
            lastColor=blend
        }
    }
    for i:=0;i<len(row); {
        if row[i]==0x1b {
            end:=ansiSequenceEnd(row,i)
            if end<=i {end=i+1}
            if row[end-1]=='m' && strings.HasPrefix(row[i:end],"\x1b[") {
                current=interrogationANSIColor(row[i+2:end-1],current,normal)
                lastColor=[3]int{-1,-1,-1} // SGR overwrites our foreground
            }
            output.WriteString(row[i:end])
            i=end
            continue
        }
        ch,size:=utf8.DecodeRuneInString(row[i:])
        if size<1 {break}
        emit()
        output.WriteString(row[i:i+size])
        advance:=lipgloss.Width(string(ch))
        if advance>0 {x+=advance}
        i+=size
    }
    // Restore the terminal's default foreground for subsequent UI cells.
    output.WriteString("\x1b[39m")
    return output.String()
}
