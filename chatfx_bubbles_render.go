package main

import (
    "fmt"
    "math"
    "strings"
    "unicode/utf8"

    "charm.land/lipgloss/v2"
)

// A braille character provides a 2x4 sub-cell grid, turning a curved rim
// into a much finer outline than ASCII slashes and pipes can achieve.
type bubblePixel struct {
    glyph rune
    color [3]int
}

var bubbleBrailleDots=[4][2]uint8{
    {0x01,0x08}, {0x02,0x10}, {0x04,0x20}, {0x40,0x80},
}

// An elliptical signed-distance outline. The signed spring displacement
// squashes one axis while stretching the other, preserving approximate area.
func bubbleBrailleMask(b bubbleParticle,x,y int) uint8 {
    if b.radius<=0 {return 0}
    // Sample the elastic radial border at each Braille subpixel. Terminal
    // character cells are roughly twice as tall as they are wide.
    rx:=2*b.radius
    ry:=b.radius
    var mask uint8
    for subY:=0;subY<4;subY++ {
        sy:=float64(y)+(float64(subY)+0.5)/4
        dy:=(sy-b.y)/ry
        for subX:=0;subX<2;subX++ {
            sx:=float64(x)+(float64(subX)+0.5)/2
            dx:=(sx-b.x)/rx
            // A narrow rim about one braille dot thick.
            dist:=math.Hypot(dx,dy)
            angle:=math.Atan2(dy,dx)
            surface:=b.rimRadius(angle)/b.radius
            thickness:=math.Max(0.028,0.24/b.radius)
            if math.Abs(dist-surface)<=thickness {
                mask|=bubbleBrailleDots[subY][subX]
            }
        }
    }
    return mask
}

func bubbleGlyphAt(b bubbleParticle,x,y int) rune {
    if b.radius<=0 {return 0}
    mask:=bubbleBrailleMask(b,x,y)
    if mask!=0 {return rune(0x2800)+rune(mask)}
    // Reflections are painted once per bubble in raster(), not evaluated
    // independently for every character (which created two little eyes).
    return 0
}

func bubbleOutlineColor(b bubbleParticle,glyph rune) [3]int {
    if glyph=='o' {return [3]int{244,250,255}}
    var color [3]int
    for c:=0;c<3;c++ {
        color[c]=min(255,int(float64(b.color[c])*0.68+85))
    }
    return color
}

// Each bubble only visits its bounded rectangle; cost remains proportional
// to the visible outlines, with a strict 32-bubble population cap.
func (s *bubbleState) raster(width,height int) [][]bubblePixel {
    grid:=make([][]bubblePixel,height)
    paint:=func(x,y int,glyph rune,color [3]int) {
        if x<0 || x>=width || y<0 || y>=height || glyph==0 {return}
        if grid[y]==nil {grid[y]=make([]bubblePixel,width)}
        grid[y][x]=bubblePixel{glyph:glyph,color:color}
    }
    for _,b:=range s.bubbles {
        rx:=2*b.radius*1.30
        ry:=b.radius*1.30
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
        // Place exactly ONE reflection cluster, attached to the bubble's
        // upper-left local surface. Large bubbles get a single curved
        // Braille highlight rather than multiple isolated 'o' glyphs.
        hx:=int(math.Round(b.x-0.92*b.rimRadius(-2.35)))
        hy:=int(math.Round(b.y-0.52*b.rimRadius(-2.35)))
        white:=[3]int{244,250,255}
        if b.radius<3.8 {
            paint(hx,hy,'o',white)
        } else {
            // A shallow, contiguous crescent; no duplicated eye highlights.
            paint(hx,hy,'⠐',white)
            paint(hx+1,hy,'⠒',white)
            if b.radius>=5.3 {paint(hx+2,hy+1,'⠂',white)}
        }
    }
    // Short-lived pop fragments fly outward with independent velocities.
    for _,p:=range s.pops {
        cx,cy:=int(math.Round(p.x)),int(math.Round(p.y))
        for _,part:=range p.fragments {
            paint(int(math.Round(part.x)),int(math.Round(part.y)),part.glyph,p.color)
        }
        if p.age<0.27 {
            for i,g:=range "*pop*" {
                paint(cx-2+i,cy,g,p.color)
            }
        }
    }
    return grid
}

func bubblePaintRow(row string,pixels []bubblePixel,themeBG string,width int) string {
    if len(pixels)==0 || width<=0 {return row}
    baseBG:=interrogationRGB(themeBG,[3]int{17,19,24})
    baseFG:=[3]int{230,230,230}
    currentBG:=baseBG
    currentFG:=baseFG
    // Never paint inside the occupied message prefix, including spaces
    // between words. Outlines live in the unused chat background.
    protectedWidth:=lipgloss.Width(strings.TrimRight(stripANSISequences(row)," "))
    var out strings.Builder
    out.Grow(len(row)+width*9)
    x:=0
    draw:=func(ch string,blank bool) {
        if x>=width {out.WriteString(ch);return}
        pixel:=pixels[x]
        if blank && x>=protectedWidth && pixel.glyph!=0 {
            fmt.Fprintf(&out,"\x1b[38;2;%d;%d;%dm%c\x1b[38;2;%d;%d;%dm",
                pixel.color[0],pixel.color[1],pixel.color[2],pixel.glyph,
                currentFG[0],currentFG[1],currentFG[2])
        } else {
            out.WriteString(ch)
        }
    }
    for i:=0;i<len(row); {
        if row[i]==0x1b {
            end:=ansiSequenceEnd(row,i)
            if end<=i {end=i+1}
            if row[end-1]=='m' && strings.HasPrefix(row[i:end],"\x1b[") {
                params:=row[i+2:end-1]
                currentBG=interrogationANSIBackground(params,currentBG,baseBG)
                currentFG=interrogationANSIColor(params,currentFG,baseFG)
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
