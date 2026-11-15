package main

// CRT static and digital glitches are deterministic, client-local ChatFX.
// Time is measured on the fixed 30 Hz clock, so /fxfps changes redraw
// frequency but not interference timing. Neither effect touches message text.

var crtStaticGlyphs = [...]rune{'.', ':', ',', '*', '+', '#', '%', '/', '\\'}
var crtStaticColors = [...]string{"#66717B", "#87919A", "#A8AFB5", "#D2D6D9", "#F1F1ED"}
var digitalGlitchGlyphs = [...]rune{'#', '%', '=', '_', '/', '\\', '|', '+', 'x', '0'}
var digitalGlitchColors = [...]string{
    "#00F5FF", "#FF38C8", "#A85CFF", "#40FF8A", "#FFDB4A", "#3D8FFF",
}

// Interference timing is made of long quiet cycles and short, uneven
// disturbances. Sampling the same tick always produces the same result,
// including after terminal resizing.
func interferenceBurst(tick uint64, salt uint64, glitch bool) (uint64, bool) {
    var cycle uint64 = 150 // 5-second static cycle
    if glitch {cycle=240} // digital errors are less frequent
    block:=tick/cycle
    frame:=tick%cycle
    seed:=chatFXHash(salt+block*0x9e3779b97f4a7c15)
    start:=uint64(8+seed%70)
    length:=uint64(4+(seed>>8)%9)
    if glitch {
        start=uint64(10+seed%115)
        length=uint64(5+(seed>>8)%12)
    }
    return seed,frame>=start && frame<start+length
}

func (m model) staticPositions(width,height int) []map[int]chatFXCell {
    if width<1||height<1 {return nil}
    rows:=make([]map[int]chatFXCell,height)
    tick:=m.fxSimSteps
    seed,active:=interferenceBurst(tick,m.fxSalt^0x57a71c,false)
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

func (m model) glitchPositions(width,height int) []map[int]chatFXCell {
    if width<1||height<1 {return nil}
    rows:=make([]map[int]chatFXCell,height)
    tick:=m.fxSimSteps
    seed,active:=interferenceBurst(tick,m.fxSalt^0x6117c4,true)
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
        color:=digitalGlitchColors[(r>>42)%uint64(len(digitalGlitchColors))]
        for x:=left;x<left+span;x++ {
            // Intentional holes make the strips look like torn data.
            h:=chatFXHash(r+uint64(x/3))
            if h%7==0 {continue}
            set(x,y,h+uint64(x),color)
        }
        if y+1<height && (r>>11)%2==0 {
            alt:=digitalGlitchColors[(r>>47)%uint64(len(digitalGlitchColors))]
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
        color:=digitalGlitchColors[(r>>51)%uint64(len(digitalGlitchColors))]
        for yy:=y;yy<y+bh;yy++ {
            for xx:=x;xx<x+bw;xx++ {
                if (xx+yy)%5==0 {continue}
                set(xx,yy,chatFXHash(r+uint64(xx)+uint64(yy)*97),color)
            }
        }
    }
    return rows
}
