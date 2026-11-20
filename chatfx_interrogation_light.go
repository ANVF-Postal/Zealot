package main

import (
    "sync"
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
    light:=0.23+cachedInterrogationLightShape(x,y,width,height)*flicker
    return math.Max(0.22,math.Min(1,light))
}

// Precompute only the stationary shape of the lamp. The time-dependent
// flicker remains outside the cache, so its colour/timing is unchanged.
type interrogationLightGeometryKey struct {width,height int}
var interrogationLightGeometry = struct {
    sync.RWMutex
    values map[interrogationLightGeometryKey][]float64
}{values:make(map[interrogationLightGeometryKey][]float64)}

func interrogationLightShape(width,height int) []float64 {
    key:=interrogationLightGeometryKey{width,height}
    interrogationLightGeometry.RLock()
    values,ok:=interrogationLightGeometry.values[key]
    interrogationLightGeometry.RUnlock()
    if !ok {
        values=make([]float64,width*height)
        cx:=float64(width-1)*0.50
        cy:=float64(height-1)-math.Min(1.5,float64(height-1)*0.15)
        rx:=math.Max(3,float64(width)*0.73)
        ry:=math.Max(4,math.Min(16,float64(height)*0.57))
        for yy:=0;yy<height;yy++ {
            dy:=(float64(yy)-cy)/ry
            verticalEdge:=math.Abs(float64(yy)-cy)/math.Max(1,float64(height-1))
            for xx:=0;xx<width;xx++ {
                dx:=(float64(xx)-cx)/rx
                glow:=math.Exp(-1.7*(dx*dx+dy*dy))
                horizontalEdge:=math.Abs(float64(xx)-cx)/math.Max(1,cx)
                rim:=1-0.34*math.Pow(horizontalEdge,4)-0.13*math.Pow(verticalEdge,4)
                values[yy*width+xx]=0.72*glow*math.Max(0.45,rim)
            }
        }
        interrogationLightGeometry.Lock()
        if existing,found:=interrogationLightGeometry.values[key];found {
            values=existing
        } else {
            if len(interrogationLightGeometry.values)>=8 {
                clear(interrogationLightGeometry.values)
            }
            interrogationLightGeometry.values[key]=values
        }
        interrogationLightGeometry.Unlock()
    }
    return values
}

func cachedInterrogationLightShape(x,y,width,height int) float64 {
    if x<0 {x=0} else if x>=width {x=width-1}
    if y<0 || y>=height {return 0}
    return interrogationLightShape(width,height)[y*width+x]
}

func interrogationWriteRGB(output *strings.Builder,layer int,color [3]int) {
    var bytes [48]byte
    data:=bytes[:0]
    data=append(data,'\x1b','[')
    data=strconv.AppendInt(data,int64(layer),10)
    data=append(data,';','2',';')
    for i,v:=range color {
        if i>0 {data=append(data,';')}
        data=strconv.AppendInt(data,int64(v),10)
    }
    data=append(data,'m')
    output.Write(data)
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
    // A single lookup per row; the per-cell lighting loop must not acquire
    // a cache lock for every character in the viewport.
    var shape []float64
    if height>0 && y>=0 && y<height {shape=interrogationLightShape(width,height)}
    var output strings.Builder
    output.Grow(len(row)+width*38)
    // Source styles can contain inverse-video attributes inherited from
    // previous effects. Start this row with known terminal attributes.
    output.WriteString("\x1b[0m")
    x:=0
    lastFG,lastBG:=[3]int{-1,-1,-1},[3]int{-1,-1,-1}
    emit:=func() {
        sample:=0.0
        if len(shape)>0 {
            sample=math.Max(0.22,math.Min(1,0.23+shape[y*width+min(width-1,x)]*flicker))
        } else {
            sample=interrogationLightAt(min(width-1,x),y,width,height,emphasis,flicker)
        }
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
            interrogationWriteRGB(&output,48,bgShade)
            lastBG=bgShade
        }
        if fgShade!=lastFG {
            interrogationWriteRGB(&output,38,fgShade)
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
            if row[end-1]=='m' && strings.HasPrefix(row[i:end],"\x1b[") {
                // Never allow a previous FX's reverse-video state to swap
                // Matrix glyph colours into full-width background bands.
                output.WriteString("\x1b[27m")
            }
            i=end
            continue
        }
        ch,size:=utf8.DecodeRuneInString(row[i:])
        if size<1 {break}
        if x<width {emit()}
        output.WriteString(row[i:i+size])
        advance:=1
        if size>1 {advance=lipgloss.Width(string(ch))}
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
