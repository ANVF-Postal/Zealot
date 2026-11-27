package main

import (
    "fmt"
    "math"
    "strings"
    "testing"
)

// A skipped Braille cell must always have had an empty mask in the original
// exact per-subpixel renderer, including for strongly deformed outlines.
func TestBubbleRimCullingNeverDropsBrailleDots(t *testing.T) {
    for _,radius:=range []float64{1.3,2.7,4.5,9.0} {
        b:=bubbleParticle{radius:radius,x:23.37,y:17.61}
        for i:=range b.rim {
            b.rim[i]=-0.40+0.58*float64((i*7)%24)/23
        }
        inner,outer:=bubbleRimBounds(b)
        for y:=0;y<45;y++ {
            for x:=0;x<80;x++ {
                if !bubbleCellMayTouchRim(b,x,y,inner,outer) && bubbleBrailleMask(b,x,y)!=0 {
                    t.Fatalf("culled occupied cell radius=%.2f at (%d,%d)",radius,x,y)
                }
            }
        }
    }
}

func referenceInterrogationLightAt(x,y,width,height int,emphasis,flicker float64) float64 {
    if width<1 || height<1 {return 1}
    cx:=float64(width-1)*0.50
    cy:=float64(height-1)-math.Min(1.5,float64(height-1)*0.15)
    rx:=math.Max(3,float64(width)*0.73)
    ry:=math.Max(4,math.Min(16,float64(height)*0.57))
    dx:=(float64(x)-cx)/rx
    dy:=(float64(y)-cy)/ry
    glow:=math.Exp(-1.7*(dx*dx+dy*dy))
    horizontalEdge:=math.Abs(float64(x)-cx)/math.Max(1,cx)
    verticalEdge:=math.Abs(float64(y)-cy)/math.Max(1,float64(height-1))
    rim:=1-0.34*math.Pow(horizontalEdge,4)-0.13*math.Pow(verticalEdge,4)
    light:=0.23+0.72*glow*math.Max(0.45,rim)*flicker
    return math.Max(0.22,math.Min(1,light))
}

func TestCachedInterrogationLampMatchesOriginalLighting(t *testing.T) {
    for _,size:=range [][2]int{{12,7},{80,24},{160,45},{320,80}} {
        width,height:=size[0],size[1]
        for _,flicker:=range []float64{0.88,0.95,1} {
            for y:=0;y<height;y++ {
                for x:=0;x<width;x++ {
                    got:=interrogationLightAt(x,y,width,height,0,flicker)
                    want:=referenceInterrogationLightAt(x,y,width,height,0,flicker)
                    if math.Abs(got-want)>1e-12 {
                        t.Fatalf("light %dx%d (%d,%d) flicker %.2f got %.14f want %.14f",width,height,x,y,flicker,got,want)
                    }
                }
            }
        }
    }
}

func TestInterrogationRGBWriterKeepsOriginalANSIBytes(t *testing.T) {
    for _,layer:=range []int{38,48} {
        for _,rgb:=range [][3]int{{0,0,0},{17,19,24},{244,250,255},{255,255,255}} {
            var b strings.Builder
            interrogationWriteRGB(&b,layer,rgb)
            want:=fmt.Sprintf("\x1b[%d;2;%d;%d;%dm",layer,rgb[0],rgb[1],rgb[2])
            if b.String()!=want {t.Fatalf("RGB ANSI mismatch: got %q want %q",b.String(),want)}
        }
    }
}

func TestAquariumCachedSpriteWidthPreservesGlyphColours(t *testing.T) {
    fish:=aquariumSwimmers(160,45,30,73)
    // Check both directions and all printable art frames, including the
    // wide multi-line predator and its per-column colour highlights.
    for _,swimmer:=range fish {
        for _,direction:=range []int{-1,1} {
            swimmer.direction=direction
            art:=aquariumFishArt(swimmer)
            width:=0
            for _,line:=range art {
                width=max(width,len([]rune(line)))
            }
            for row,line:=range art {
                for col,glyph:=range line {
                    got:=aquariumFishGlyphColorWithWidth(swimmer,row,col,glyph,width)
                    want:=aquariumFishGlyphColor(swimmer,row,col,glyph)
                    if got!=want {
                        t.Fatalf("fish colour changed at %d,%d: got %q want %q",row,col,got,want)
                    }
                }
            }
        }
    }
}

func TestTerminalRevealDurationCacheMatchesTypingTimeline(t *testing.T) {
    for _,text:=range []string{"Hello.","What's happening?!","A long statement with spaces and punctuation...","界 hello!"} {
        duration:=terminalTextDuration(text)
        cached:=terminalReveal{duration:duration,text:text}
        legacy:=terminalReveal{text:text}
        if terminalRevealDuration(cached)!=duration || terminalRevealDuration(legacy)!=duration {
            t.Fatalf("terminal duration cache changed punctuation timing for %q",text)
        }
    }
}
