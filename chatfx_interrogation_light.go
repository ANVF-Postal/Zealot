package main

import (
    "fmt"
    "math"
    "strings"
    "strconv"
    "unicode/utf8"

    "charm.land/lipgloss/v2"
)

// The lamp is just above the bottom of the full chat pane, including typing.
// An elliptical Gaussian produces a rounded vignette in terminal cells.
// Wider light pool with a deeper rim; older messages remain decipherable.
func interrogationLightAt(x,y,width,height int, emphasis,flicker float64) float64 {
    if width<1 || height<1 {return 1}
    cx:=float64(width-1)*0.50
    cy:=float64(height-1)-math.Min(1.5,float64(height-1)*0.15)
    rx:=math.Max(3,float64(width)*0.73)
    ry:=math.Max(4,math.Min(16,float64(height)*0.57))
    dx:=(float64(x)-cx)/rx
    dy:=(float64(y)-cy)/ry
    glow:=math.Exp(-1.7*(dx*dx+dy*dy))
    // Larger pool, with extra edge attenuation to retain a dramatic rim.
    horizontalEdge:=math.Abs(float64(x)-cx)/math.Max(1,cx)
    verticalEdge:=math.Abs(float64(y)-cy)/math.Max(1,float64(height-1))
    rim:=1-0.34*math.Pow(horizontalEdge,4)-0.13*math.Pow(verticalEdge,4)
    light:=0.23+0.72*glow*math.Max(0.45,rim)*flicker
    // No message-specific brightness bands: the pool stays fixed while
    // chat lines scroll through it, avoiding abrupt horizontal seams.
    return math.Max(0.22,math.Min(1,light))
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

// The light is a colour transform of BOTH terminal layers. Every cell gets
// a themed foreground and a shaded background, including blank padding cells.
// We never change printable text or the history cache; the scrollbar is added
// only after this function has finished painting the chat content.
func interrogationLightRow(row,background,foreground string,width,height,y int,amount,emphasis,flicker float64) string {
    if amount<=0 || width<1 {return row}
    bg:=interrogationRGB(background,[3]int{17,19,24})
    normal:=interrogationRGB(foreground,[3]int{230,230,230})
    currentFG,currentBG:=normal,bg
    var output strings.Builder
    output.Grow(len(row)+width*38)
    x:=0
    lastFG,lastBG:=[3]int{-1,-1,-1},[3]int{-1,-1,-1}
    emit:=func() {
        sample:=interrogationLightAt(min(width-1,x),y,width,height,emphasis,flicker)
        // Use 32 light levels to keep wide terminal panes from producing
        // two truecolour ANSI sequences per cell on every frame.
        sample=math.Round(sample*32)/32
        // Do not extinguish contrast: old text stays challenging but legible.
        textOpacity:=1-amount*(1-sample)
        // Even the empty background receives the vignette; the lamp is
        // slightly darker than normal at its centre and darkest at the rim.
        backgroundStrength:=1-amount*(1-(0.22+0.74*sample))
        textOpacity=math.Round(textOpacity*64)/64
        backgroundStrength=math.Round(backgroundStrength*64)/64
        fgShade,bgShade:=[3]int{},[3]int{}
        for c:=0;c<3;c++ {
            // Foreground fades into the locally lit background, not a flat
            // unlit theme colour. All RGB colours keep their relative hues.
            shadedBackground:=float64(currentBG[c])*backgroundStrength
            bgShade[c]=int(math.Round(shadedBackground))
            fgShade[c]=int(math.Round(shadedBackground+
                (float64(currentFG[c])-float64(currentBG[c]))*textOpacity))
            fgShade[c]=max(0,min(255,fgShade[c]))
        }
        if bgShade!=lastBG {
            fmt.Fprintf(&output,"\x1b[48;2;%d;%d;%dm",bgShade[0],bgShade[1],bgShade[2])
            lastBG=bgShade
        }
        if fgShade!=lastFG {
            fmt.Fprintf(&output,"\x1b[38;2;%d;%d;%dm",fgShade[0],fgShade[1],fgShade[2])
            lastFG=fgShade
        }
    }
    for i:=0;i<len(row); {
        if row[i]==0x1b {
            end:=ansiSequenceEnd(row,i)
            if end<=i {end=i+1}
            if row[end-1]=='m' && strings.HasPrefix(row[i:end],"\x1b[") {
                params:=row[i+2:end-1]
                currentFG=interrogationANSIColor(params,currentFG,normal)
                currentBG=interrogationANSIBackground(params,currentBG,bg)
                // An SGR reset or colour setter overrides both our styles.
                lastFG,lastBG=[3]int{-1,-1,-1},[3]int{-1,-1,-1}
            }
            output.WriteString(row[i:end])
            i=end
            continue
        }
        ch,size:=utf8.DecodeRuneInString(row[i:])
        if size<1 {break}
        if x<width {emit()}
        output.WriteString(row[i:i+size])
        advance:=lipgloss.Width(string(ch))
        if advance>0 {x+=advance}
        i+=size
    }
    // Rows padded by lipgloss are normally already full width. If a row is
    // shorter, explicitly paint the remaining blank cells so the vignette
    // covers the whole chat pane instead of only the occupied text.
    for x<width {
        emit()
        output.WriteByte(' ')
        x++
    }
    // Do not carry dimmed backgrounds into the scrollbar or typing line.
    output.WriteString("\x1b[0m")
    return output.String()
}

func interrogationANSIBackground(parameters string,current,normal [3]int) [3]int {
    fields:=strings.Split(parameters,";")
    for i:=0;i<len(fields);i++ {
        n,err:=strconv.Atoi(fields[i])
        if err!=nil {continue}
        switch {
        case n==0 || n==49:
            current=normal
        case n==48 && i+4<len(fields) && fields[i+1]=="2":
            r,e1:=strconv.Atoi(fields[i+2])
            g,e2:=strconv.Atoi(fields[i+3])
            b,e3:=strconv.Atoi(fields[i+4])
            if e1==nil && e2==nil && e3==nil {
                current=[3]int{max(0,min(255,r)),max(0,min(255,g)),max(0,min(255,b))}
            }
            i+=4
        case n==48 && i+2<len(fields) && fields[i+1]=="5":
            index,e:=strconv.Atoi(fields[i+2])
            if e==nil && index>=0 && index<=255 {current=interrogationANSI256(index)}
            i+=2
        case n>=40 && n<=47:
            current=interrogationANSI256(n-40)
        case n>=100 && n<=107:
            current=interrogationANSI256(n-100+8)
        }
    }
    return current
}
