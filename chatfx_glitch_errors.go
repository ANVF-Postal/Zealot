package main

import (
    "fmt"
    "strconv"
)

// These are imaginary diagnostics, not real host information. Their timing
// depends only on the local fixed-step ChatFX clock and salt.
var glitchErrorMessages = [...]string{
    "ERR: frame desync at /dev/tty7",
    "panic: invalid raster pointer",
    "EIO: /var/cache/signal/ghost.bin",
    "warning: checksum mismatch",
    "segfault at 0x%08x",
    "kernel: corrupted page table",
    "fsck /mnt/void: bad superblock",
    "signal lost // retrying...",
    "fault: /sys/video/buffer.map",
    "write failed: sector 0x%06x",
    "TRACE: renderer.sync() -> -1",
    "FATAL: /tmp/ghost/frame_%03d",
    "memory integrity: FAILED",
    "ERR 0x%08x // retry",
}

type glitchErrorEvent struct {
    start, duration uint64
    seed uint64
}

// Random-looking waits, but with a fixed, reproducible event stream.
// Typical starts are separated by roughly 9-33 seconds.
func glitchTextEvent(tick, salt uint64) (glitchErrorEvent,bool) {
    next:=uint64(0)
    for i:=uint64(0);i<100000;i++ {
        seed:=chatFXHash(salt^0x7e827aa2+(i+1)*0x9e3779b97f4a7c15)
        next+=270+(seed>>8)%720
        duration:=glitchErrorDuration(seed) // writing, brief hold, fade
        if tick<next {return glitchErrorEvent{},false}
        if tick<next+duration {
            return glitchErrorEvent{start:next,duration:duration,seed:seed},true
        }
        next+=duration
    }
    return glitchErrorEvent{},false
}

// Pause the ordinary visual corruption clock whenever text diagnostics own
// the screen, rather than skipping a scheduled streak behind an error block.
func glitchDistortionTick(tick, salt uint64) uint64 {
    next:=uint64(0)
    paused:=uint64(0)
    for i:=uint64(0);i<100000;i++ {
        seed:=chatFXHash(salt^0x7e827aa2+(i+1)*0x9e3779b97f4a7c15)
        next+=270+(seed>>8)%720
        if tick<=next {break}
        duration:=glitchErrorDuration(seed)
        elapsed:=min(duration,tick-next)
        paused+=elapsed
        if tick<next+duration {break}
        next+=duration
    }
    return tick-paused
}

func glitchErrorDuration(seed uint64) uint64 {
    lines:=2+int((seed>>43)%5)
    speed:=uint64(1+(seed>>51)%2)
    writing:=uint64(0)
    for i:=0;i<lines;i++ {
        writing+=uint64(len(glitchErrorLine(seed,i)))*speed+7
    }
    return writing+45+32 // all lines complete, linger, then fade
}

func glitchErrorLine(seed uint64, line int) string {
    h:=chatFXHash(seed+uint64(line+1)*0xbf58476d1ce4e5b9)
    if line==0 || h%3==0 {
        return fmt.Sprintf("0x%07x",uint32(h&0x0fffffff))
    }
    msg:=glitchErrorMessages[int(h%uint64(len(glitchErrorMessages)))]
    switch {
    case msg=="segfault at 0x%08x" || msg=="write failed: sector 0x%06x" || msg=="ERR 0x%08x // retry":
        return fmt.Sprintf(msg,uint32(chatFXHash(h+1)))
    case msg=="FATAL: /tmp/ghost/frame_%03d":
        return fmt.Sprintf(msg,int(h%1000))
    default:
        return msg
    }
}

// Muted text uses the classic PuTTY ANSI RGB values blended toward the chat
// theme background. Numeric or unparseable theme colours use a dark fallback.
func glitchMutedColor(hexColor, background string, fade float64) string {
    parse:=func(s string, fallback [3]int) [3]int {
        if len(s)!=7 || s[0]!='#' {return fallback}
        var result [3]int
        for i:=0;i<3;i++ {
            n,err:=strconv.ParseUint(s[1+i*2:3+i*2],16,8)
            if err!=nil {return fallback}
            result[i]=int(n)
        }
        return result
    }
    fg:=parse(hexColor,[3]int{100,100,100})
    bg:=parse(background,[3]int{17,19,24})
    if fade<0 {fade=0}
    if fade>1 {fade=1}
    // Even at its peak, errors are background decoration, never full contrast.
    opacity:=0.44*fade
    return fmt.Sprintf("#%02X%02X%02X",
        int(float64(bg[0])*(1-opacity)+float64(fg[0])*opacity),
        int(float64(bg[1])*(1-opacity)+float64(fg[1])*opacity),
        int(float64(bg[2])*(1-opacity)+float64(fg[2])*opacity))
}

func (m model) glitchErrorPositions(width,height int) ([]map[int]chatFXCell,bool) {
    if width<1 || height<1 {return nil,false}
    event,active:=glitchTextEvent(m.fxSimSteps,m.fxSalt)
    if !active {return nil,false}
    rows:=make([]map[int]chatFXCell,height)
    age:=m.fxSimSteps-event.start
    // A few lines build into a block, each typing left to right.
    lines:=2+int((event.seed>>43)%5)
    lines=min(lines,height)
    // Use an aligned error block, but keep it away from the extreme edges.
    x0:=0
    if width>32 {x0=2+int((event.seed>>9)%uint64(max(1,width-30)))}
    y0:=int((event.seed>>17)%uint64(max(1,height-lines+1)))
    speed:=uint64(1+(event.seed>>51)%2) // 15-30 chars per second
    cursor:='▌'
    // A short fade at the end is reserved so no other glitches appear while
    // terminal error text is still disappearing.
    fade:=1.0
    fadeTicks:=uint64(32)
    if age>event.duration-fadeTicks {
        fade=float64(event.duration-age)/float64(fadeTicks)
    }
    for line:=0;line<lines;line++ {
        message:=glitchErrorLine(event.seed,line)
        // Each subsequent diagnostic waits for its predecessor to finish.
        before:=uint64(0)
        for prev:=0;prev<line;prev++ {
            before+=uint64(len(glitchErrorLine(event.seed,prev)))*speed+7
        }
        if age<before {break}
        progress:=(age-before)/speed
        count:=min(len(message),int(progress))
        if count<0 {count=0}
        y:=y0+line
        if y>=height {break}
        shade:=digitalGlitchColors[int(chatFXHash(event.seed+uint64(line)*17)%uint64(len(digitalGlitchColors)))]
        color:=glitchMutedColor(shade,m.cfg.Theme.Background,fade)
        put:=func(x int,glyph rune) {
            if x<0||x>=width || glyph==' ' {return}
            if rows[y]==nil {rows[y]=make(map[int]chatFXCell)}
            rows[y][x]=chatFXCell{glyph:glyph,color:color}
        }
        for col:=0;col<count && x0+col<width;col++ {
            put(x0+col,rune(message[col]))
        }
        // Active line shows a blinking terminal cursor; it stops when the
        // line finishes or the block enters its fade-out.
        if count<len(message) && age<event.duration-fadeTicks && (age/7)%2==0 {
            put(x0+count,cursor)
        }
    }
    return rows,true
}
