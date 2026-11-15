package main

import (
    "fmt"
    "math"
    "strings"
    "unicode/utf8"

    "charm.land/lipgloss/v2"
)

// A bubble is a thin ANSI-coloured ASCII outline with a tiny glint. Its
// interior is genuinely empty: the theme background and chat remain visible.
type bubblePixel struct {
    glyph byte
    color [3]int
}

func bubbleGlyphAt(b bubbleParticle,x,y int) byte {
    dx:=(float64(x)+0.5-b.x)/(2*b.radius)
    dy:=(float64(y)+0.5-b.y)/b.radius
    // Tiny damped deformation creates a soft squish after collisions without
    // compromising the roughly circular Windows screensaver silhouette.
    dx*=1+b.wobble
    dy*=1-b.wobble*0.5
    distance:=math.Hypot(dx,dy)
    thickness:=math.Max(0.11,0.37/b.radius)
    if math.Abs(distance-1)<=thickness {
        switch {
        case math.Abs(dy)>0.82: return '-'
        case math.Abs(dx)>0.83: return '|'
        case dx*dy>0: return '\\'
        default: return '/'
        }
    }
    // Single round specular reflection in the upper-left quadrant.
    if math.Abs(dx+0.43)<0.14 && math.Abs(dy+0.48)<0.19 {
        return 'o'
    }
    return 0
}

func bubbleOutlineColor(b bubbleParticle,glyph byte) [3]int {
    if glyph=='o' {return [3]int{244,250,255}}
    // A slightly pale, iridescent outline against the normal chat background.
    var color [3]int
    for c:=0;c<3;c++ {
        color[c]=min(255,int(float64(b.color[c])*0.68+85))
    }
    return color
}

// Rasterize outlines rather than coloured surfaces. Existing message cells
// are left intact during painting, so the animation never overwrites chat.
func (s *bubbleState) raster(width,height int) [][]bubblePixel {
    grid:=make([][]bubblePixel,height)
    paint:=func(x,y int,glyph byte,color [3]int) {
        if x<0 || x>=width || y<0 || y>=height || glyph==0 {return}
        if grid[y]==nil {grid[y]=make([]bubblePixel,width)}
        grid[y][x]=bubblePixel{glyph:glyph,color:color}
    }
    for _,b:=range s.bubbles {
        rx:=2*b.radius*(1+b.wobble)
        ry:=b.radius*(1+b.wobble)
        startX:=max(0,int(math.Floor(b.x-rx))-1)
        endX:=min(width,int(math.Ceil(b.x+rx))+1)
        startY:=max(0,int(math.Floor(b.y-ry))-1)
        endY:=min(height,int(math.Ceil(b.y+ry))+1)
        for y:=startY;y<endY;y++ {
            for x:=startX;x<endX;x++ {
                glyph:=bubbleGlyphAt(b,x,y)
                if glyph!=0 {paint(x,y,glyph,bubbleOutlineColor(b,glyph))}
            }
        }
    }
    for _,p:=range s.pops {
        cx,cy:=int(math.Round(p.x)),int(math.Round(p.y))
        if p.age<0.42 {
            // Burst expands briefly while the sphere vanishes immediately.
            spread:=1+int(p.age/0.12)
            paint(cx-2*spread,cy,'*',p.color)
            paint(cx+2*spread,cy,'*',p.color)
            paint(cx,cy-spread,'+',p.color)
            paint(cx,cy+spread,'+',p.color)
        }
        if p.age<0.55 {
            for i,g:=range []byte("*pop*") {
                paint(cx-2+i,cy,g,p.color)
            }
        }
    }
    return grid
}

func bubblePaintRow(row string,pixels []bubblePixel,themeBG string,width int) string {
    if len(pixels)==0 || width<=0 {return row}
    baseBG:=interrogationRGB(themeBG,[3]int{17,19,24})
    currentBG:=baseBG
    var out strings.Builder
    out.Grow(len(row)+width*9)
    x:=0
    draw:=func(ch string,blank bool) {
        if x>=width {out.WriteString(ch);return}
        pixel:=pixels[x]
        if blank && pixel.glyph!=0 {
            fmt.Fprintf(&out,"\x1b[38;2;%d;%d;%dm%c\x1b[39m",
                pixel.color[0],pixel.color[1],pixel.color[2],pixel.glyph)
        } else {
            out.WriteString(ch)
        }
    }
    for i:=0;i<len(row); {
        if row[i]==0x1b {
            end:=ansiSequenceEnd(row,i)
            if end<=i {end=i+1}
            if row[end-1]=='m' && strings.HasPrefix(row[i:end],"\x1b[") {
                currentBG=interrogationANSIBackground(row[i+2:end-1],currentBG,baseBG)
            }
            out.WriteString(row[i:end])
            i=end
            continue
        }
        ch,size:=utf8.DecodeRuneInString(row[i:])
        if size<=0 {break}
        w:=lipgloss.Width(string(ch))
        if w==1 {
            // Coloured mention backgrounds are respected; only actual blank
            // cells against the chat background receive the ASCII outlines.
            blank:=ch==' ' && currentBG==baseBG
            draw(row[i:i+size],blank)
            x++
        } else {
            out.WriteString(row[i:i+size])
            if w>0 {x+=w}
        }
        i+=size
    }
    for x<width {
        draw(" ",true)
        x++
    }
    // Source styling remains untouched; prevent an outline from spilling
    // into the scrollbar to the right of the chat-content column.
    out.WriteString("\x1b[0m")
    return out.String()
}

func (m model) renderBubblesRows(rows []string,width int) {
    if m.fxBubbles==nil || len(rows)==0 || width<=0 {return}
    grid:=m.fxBubbles.raster(width,len(rows))
    for y,pixels:=range grid {
        if len(pixels)==0 {continue}
        rows[y]=bubblePaintRow(rows[y],pixels,m.cfg.Theme.Background,width)
    }
}
