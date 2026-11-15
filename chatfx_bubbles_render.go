package main

import (
    "fmt"
    "math"
    "strings"
    "unicode/utf8"

    "charm.land/lipgloss/v2"
)

type bubbleShade struct {
    color [3]int
    alpha float64
}

// Rasterize a glossy, subtly transparent, three-dimensional sphere. The
// terminal's roughly 2:1 character aspect ratio is corrected in the physics
// and render geometry. Lighting combines a coloured body, a thin bright rim,
// a soft specular highlight above-left, and a darker bottom-right surface.
func bubbleShadeAt(b bubbleParticle,x,y int) (bubbleShade,bool) {
    radius:=b.radius
    if b.age<0.55 {radius*=0.2+0.8*math.Min(1,b.age/0.55)}
    if remaining:=b.lifetime-b.age;remaining<0.4 {
        radius*=math.Max(0,remaining/0.4)
    }
    if radius<0.08 {return bubbleShade{},false}
    dx:=(float64(x)+0.5-b.x)/(radius*2)
    dy:=(float64(y)+0.5-b.y)/radius
    r2:=dx*dx+dy*dy
    if r2>=1 {return bubbleShade{},false}
    distance:=math.Sqrt(r2)
    rim:=math.Pow(distance,5)
    highlight:=math.Exp(-((dx+0.34)*(dx+0.34)*28+(dy+0.40)*(dy+0.40)*28))
    glimmer:=math.Exp(-((dx+0.15)*(dx+0.15)*12+(dy+0.25)*(dy+0.25)*14))
    shadow:=math.Max(0,dx+dy)*0.25
    alpha:=0.18+0.39*rim+0.20*glimmer
    alpha=math.Max(0.12,math.Min(0.87,alpha))
    var color [3]int
    for c:=0;c<3;c++ {
        value:=float64(b.color[c])*(1-shadow)+
            (255-float64(b.color[c]))*(0.70*highlight+0.18*glimmer)
        color[c]=max(0,min(255,int(math.Round(value))))
    }
    return bubbleShade{color:color,alpha:alpha},true
}

func bubbleComposite(background [3]int,shade bubbleShade) [3]int {
    result:=[3]int{}
    for c:=0;c<3;c++ {
        result[c]=int(math.Round(float64(background[c])*(1-shade.alpha)+float64(shade.color[c])*shade.alpha))
    }
    return result
}

// Draw the bubbles to a small viewport-sized shade grid first, avoiding
// a bubble-count scan for every character rendered. Each bubble only visits
// its own bounding rectangle; the 32-bubble hard cap bounds collisions too.
func (s *bubbleState) raster(width,height int) [][]bubbleShade {
    grid:=make([][]bubbleShade,height)
    for _,b:=range s.bubbles {
        r:=b.radius
        if b.age<0.55 {r*=0.2+0.8*math.Min(1,b.age/0.55)}
        if remaining:=b.lifetime-b.age;remaining<0.4 {r*=math.Max(0,remaining/0.4)}
        startX:=max(0,int(math.Floor(b.x-2*r)))
        endX:=min(width,int(math.Ceil(b.x+2*r)))
        startY:=max(0,int(math.Floor(b.y-r)))
        endY:=min(height,int(math.Ceil(b.y+r)))
        for y:=startY;y<endY;y++ {
            if grid[y]==nil {grid[y]=make([]bubbleShade,width)}
            for x:=startX;x<endX;x++ {
                shade,ok:=bubbleShadeAt(b,x,y)
                if !ok {continue}
                // Rare overlap from collision separation is composited.
                previous:=grid[y][x]
                if previous.alpha>0 {
                    shade.color=bubbleComposite(previous.color,shade)
                    shade.alpha=math.Min(1,previous.alpha+shade.alpha*(1-previous.alpha))
                }
                grid[y][x]=shade
            }
        }
    }
    return grid
}

// Bubble art is a background treatment. Original chat characters and their
// foreground colours remain intact, including wide runes, mentions and ANSI
// styling. We only add 24-bit background escape sequences around bubble cells.
func bubblePaintRow(row string, shades []bubbleShade, themeBG string,width int) string {
    if len(shades)==0 {return row}
    originalBG:=interrogationRGB(themeBG,[3]int{17,19,24})
    currentBG:=originalBG
    lastBG:=[3]int{-1,-1,-1}
    var out strings.Builder
    out.Grow(len(row)+width*14)
    x:=0
    emit:=func() {
        if x>=width {return}
        color:=currentBG
        if shades[x].alpha>0 {color=bubbleComposite(currentBG,shades[x])}
        if color!=lastBG {
            fmt.Fprintf(&out,"\x1b[48;2;%d;%d;%dm",color[0],color[1],color[2])
            lastBG=color
        }
    }
    for i:=0;i<len(row); {
        if row[i]==0x1b {
            end:=ansiSequenceEnd(row,i)
            if end<=i {end=i+1}
            if row[end-1]=='m' && strings.HasPrefix(row[i:end],"\x1b[") {
                currentBG=interrogationANSIBackground(row[i+2:end-1],currentBG,originalBG)
                lastBG=[3]int{-1,-1,-1}
            }
            out.WriteString(row[i:end])
            i=end
            continue
        }
        ch,size:=utf8.DecodeRuneInString(row[i:])
        if size<=0 {break}
        emit()
        out.WriteString(row[i:i+size])
        cellWidth:=lipgloss.Width(string(ch))
        if cellWidth>0 {x+=cellWidth}
        i+=size
    }
    for x<width {
        emit()
        out.WriteByte(' ')
        x++
    }
    out.WriteString("\x1b[0m")
    return out.String()
}

func (m model) renderBubblesRows(rows []string,width int) {
    if m.fxBubbles==nil || len(rows)==0 || width<=0 {return}
    grid:=m.fxBubbles.raster(width,len(rows))
    for y,shades:=range grid {
        if len(shades)==0 {continue}
        rows[y]=bubblePaintRow(rows[y],shades,m.cfg.Theme.Background,width)
    }
}
