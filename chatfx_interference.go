package main

// CRT static and digital glitches are deterministic, client-local ChatFX.
// Time is measured on the fixed 30 Hz clock, so /fxfps changes redraw
// frequency but not interference timing. Neither effect touches message text.

var crtStaticGlyphs = [...]rune{'.', ':', ',', '*', '+', '#', '%', '/', '\\'}
var crtStaticColors = [...]string{"#66717B", "#87919A", "#A8AFB5", "#D2D6D9", "#F1F1ED"}
// Include visible control-code symbols (␀ represents NUL, not a raw
// terminal control byte) and block graphics for stronger corruption.
var digitalGlitchGlyphs = [...]rune{'#', '%', '=', '_', '/', '\\', '|', '+', 'x', '0', '█', '▓', '▒', '░', '␀', '¤', '¬', '╬', '╳', '@', '^', '~', '?'}
// Classic PuTTY ANSI 16-colour palette, including standard and bright
// variants. Black is retained for completeness but not sampled by the
// visible glitches: it would disappear on the usual dark chat backgrounds.
var digitalGlitchColors = [...]string{
    "#000000", "#BB0000", "#00BB00", "#BBBB00",
    "#0000BB", "#BB00BB", "#00BBBB", "#BBBBBB",
    "#555555", "#FF5555", "#55FF55", "#FFFF55",
    "#5555FF", "#FF55FF", "#55FFFF", "#FFFFFF",
}

// Major events use an irregular renewal schedule rather than fixed cycles.
// The gap is measured AFTER each burst: CRT has a 3..1800 tick (0.1..60s)
// range. Most gaps are short, but occasional long outages remain possible.
// The schedule is a pure function of the simulation tick and client salt.
func interferenceGap(seed uint64, glitch bool) uint64 {
    r:=chatFXHash(seed+1)
    if glitch {
        if r%4==0 {return 20+(r>>12)%880}
        return 10+(r>>12)%220
    }
    if r%4==0 {return 3+(r>>12)%1798}
    return 3+(r>>12)%240
}

func interferenceBurst(tick uint64, salt uint64, glitch bool) (uint64,bool) {
    return cachedInterferenceBurst(tick,salt,glitch)
}

// A soft triangular envelope allows quiet minor faults to appear and fade
// rather than switching permanently between full noise and an empty screen.
func interferencePulse(tick, salt, cycle uint64) float64 {
    period:=tick/cycle
    seed:=chatFXHash(salt+period*0xbf58476d1ce4e5b9)
    offset:=uint64(2+seed%max(uint64(1),cycle/4))
    length:=uint64(12+(seed>>12)%max(uint64(1),cycle/2))
    pos:=tick%cycle
    if pos<offset || pos>=offset+length {return 0}
    progress:=float64(pos-offset)/float64(length)
    if progress<0.5 {return progress*2}
    return (1-progress)*2
}

// A minor CRT wash lasts 2–8 seconds at the fixed 30 Hz clock. Keep
// the longer, gradual fade separate from the short digital glitch pulse.
// Each deterministic 11-second window has a quiet interval, so static
// never becomes a permanent overlay.
func minorStaticPulse(tick,salt uint64) float64 {
    const cycle uint64=330
    seed:=chatFXHash(salt+(tick/cycle)*0xbf58476d1ce4e5b9)
    offset:=uint64(10+seed%35)
    length:=uint64(60+(seed>>12)%181) // 60..240 ticks = 2..8s
    pos:=tick%cycle
    if pos<offset || pos>=offset+length {return 0}
    progress:=float64(pos-offset)/float64(length)
    if progress<0.5 {return progress*2}
    return (1-progress)*2
}

func (m model) staticPositions(width,height int) []map[int]chatFXCell {
    if width<1||height<1 {return nil}
    rows:=make([]map[int]chatFXCell,height)
    tick:=m.fxSimSteps
    seed,active:=interferenceBurst(tick,m.fxSalt^0x57a71c,false)
    // Minor CRT instability continues between large interruptions: dim
    // point noise fades softly, and occasional single-frame stars pop up.
    // Only major bursts use the dense mixed-glyph screen static below.
    minor:=minorStaticPulse(tick,m.fxSalt^0x527a71)
    minorSeed:=chatFXHash(m.fxSalt+tick*0x517cc1b727220a95)
    dots:=min(85,int(float64(width*height)*0.012*minor))
    for i:=0;i<dots;i++ {
        r:=chatFXHash(minorSeed+uint64(i+1)*0x9e3779b97f4a7c15)
        x:=int(r%uint64(width))
        y:=int((r>>21)%uint64(height))
        if rows[y]==nil {rows[y]=make(map[int]chatFXCell)}
        shade:=crtStaticColors[int((r>>45)%3)]
        rows[y][x]=chatFXCell{glyph:'.',color:shade}
    }
    // Star-like noise is occasional and separate from the dot pulse.
    if minorSeed%17==0 {
        x:=int((minorSeed>>8)%uint64(width))
        y:=int((minorSeed>>30)%uint64(height))
        if rows[y]==nil {rows[y]=make(map[int]chatFXCell)}
        rows[y][x]=chatFXCell{glyph:'*',color:"#C3CACF"}
    }
    if !active {return rows}
    // A few interrupted scanlines plus random grain. Keep the count bounded
    // for large SSH sessions, and vary individual noise every 30 Hz tick.
    frameSeed:=chatFXHash(seed+tick*0xbf58476d1ce4e5b9)
    set:=func(x,y int,hash uint64) {
        if x<0||x>=width||y<0||y>=height {return}
        if rows[y]==nil {rows[y]=make(map[int]chatFXCell)}
        rows[y][x]=chatFXCell{
            glyph:crtStaticGlyphs[hash%uint64(len(crtStaticGlyphs))],
            color:crtStaticColors[(hash>>12)%uint64(len(crtStaticColors))],
        }
    }
    count:=min(650,max(12,width*height/13))
    for i:=0;i<count;i++ {
        r:=chatFXHash(frameSeed+uint64(i+1)*0x9e3779b97f4a7c15)
        set(int(r%uint64(width)),int((r>>24)%uint64(height)),r>>8)
    }
    lines:=1+int((frameSeed>>18)%3)
    for i:=0;i<lines;i++ {
        r:=chatFXHash(frameSeed+uint64(i+1)*0x517cc1b727220a95)
        y:=int(r%uint64(height))
        from:=int((r>>17)%uint64(width))
        span:=min(width-from,max(2,width/3))
        for x:=from;x<from+span;x+=2 {
            h:=chatFXHash(r+uint64(x))
            set(x,y,h)
        }
    }
    return rows
}

// Minor digital faults use a more varied ASCII punctuation set than their
// occasional underscore-only horizontal tracking tears.
var minorDigitalGlitchGlyphs = [...]rune{'_', ':', '|', '.', ',', '!', '^', '*', '\'', ';', '?', '~', '/', '\\', '+', '=', '#', '%'}

func (m model) glitchPositions(width,height int) []map[int]chatFXCell {
    if width<1||height<1 {return nil}
    // Written-out terminal errors own the display until fully faded.
    // Return immediately: no major glitches OR minor speckles can coexist.
    if rows,active:=m.glitchErrorPositions(width,height);active {return rows}
    rows:=make([]map[int]chatFXCell,height)
    // Suspended across error blocks, so regular glitches resume afterward.
    tick:=glitchDistortionTick(m.fxSimSteps,m.fxSalt)
    seed,active:=interferenceBurst(tick,m.fxSalt^0x6117c4,true)
    // Minor faults follow irregular, independently timed swells with
    // different attack/release speeds rather than a repeating pulse.
    // They never form a permanent full-screen noise field.
    minor:=minorGlitchPulse(tick,m.fxSalt^0x91ac71)
    microSeed:=chatFXHash(m.fxSalt+(tick/2)*0x517cc1b727220a95)
    speckles:=min(20,int(1+float64(width*height)*0.003*minor))
    if minor>0 {
        for i:=0;i<speckles;i++ {
            r:=chatFXHash(microSeed+uint64(i+1)*0x9e3779b97f4a7c15)
            x:=int(r%uint64(width))
            y:=int((r>>25)%uint64(height))
            if rows[y]==nil {rows[y]=make(map[int]chatFXCell)}
            rows[y][x]=chatFXCell{
                glyph:minorDigitalGlitchGlyphs[int((r>>44)%uint64(len(minorDigitalGlitchGlyphs)))],
                color:digitalGlitchColors[int((r>>48)%uint64(len(digitalGlitchColors)-1)+1)],
            }
        }
        if microSeed%3==0 {
            y:=int((microSeed>>18)%uint64(height))
            x:=int((microSeed>>31)%uint64(width))
            span:=min(width-x,max(1,width/12))
            color:=digitalGlitchColors[int((microSeed>>47)%uint64(len(digitalGlitchColors)-1)+1)]
            if rows[y]==nil {rows[y]=make(map[int]chatFXCell)}
            for dx:=0;dx<span;dx++ {
                if dx%4==3 {continue}
                rows[y][x+dx]=chatFXCell{glyph:'_',color:color}
            }
        }
    }
    if !active {return rows}
    // Horizontal torn strips and little rectangular data blocks. Stable
    // for two physics ticks to look like a damaged video frame, not rain.
    frame:=tick/2
    frameSeed:=chatFXHash(seed+frame*0x94d049bb133111eb)
    set:=func(x,y int,r uint64,color string) {
        if x<0||x>=width||y<0||y>=height {return}
        if rows[y]==nil {rows[y]=make(map[int]chatFXCell)}
        rows[y][x]=chatFXCell{
            glyph:digitalGlitchGlyphs[r%uint64(len(digitalGlitchGlyphs))],
            color:color,
        }
    }
    bands:=2+int(frameSeed%4)
    for band:=0;band<bands;band++ {
        r:=chatFXHash(frameSeed+uint64(band+1)*0xbf58476d1ce4e5b9)
        y:=int(r%uint64(height))
        left:=int((r>>15)%uint64(width))
        span:=min(width-left,max(2,width/4+int((r>>28)%uint64(max(1,width/3)))))
        color:=digitalGlitchColors[(r>>42)%uint64(len(digitalGlitchColors)-1)+1]
        for x:=left;x<left+span;x++ {
            // Intentional holes make the strips look like torn data.
            h:=chatFXHash(r+uint64(x/3))
            if h%7==0 {continue}
            set(x,y,h+uint64(x),color)
        }
        if y+1<height && (r>>11)%2==0 {
            alt:=digitalGlitchColors[(r>>47)%uint64(len(digitalGlitchColors)-1)+1]
            for x:=left;x<left+span;x+=3 {
                set(x,y+1,chatFXHash(r+uint64(x)),alt)
            }
        }
    }
    // Narrow blocks interrupt the long horizontal streaks.
    for block:=0;block<3;block++ {
        r:=chatFXHash(frameSeed+uint64(block+1)*0x517cc1b727220a95)
        x:=int(r%uint64(width))
        y:=int((r>>20)%uint64(height))
        bw:=min(width-x,2+int((r>>36)%7))
        bh:=min(height-y,1+int((r>>44)%3))
        color:=digitalGlitchColors[(r>>51)%uint64(len(digitalGlitchColors)-1)+1]
        for yy:=y;yy<y+bh;yy++ {
            for xx:=x;xx<x+bw;xx++ {
                if (xx+yy)%5==0 {continue}
                set(xx,yy,chatFXHash(r+uint64(xx)+uint64(yy)*97),color)
            }
        }
    }
    return rows
}
