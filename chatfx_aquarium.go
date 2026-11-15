package main

import "math"

// Aquarium is deliberately a lightweight, stateless ChatFX scene. Each
// client uses its usual deterministic salt and 30 Hz animation clock.
// Nothing additional needs to be sent over the room connection.
var aquariumFishColors = [...]string{
    "#F5C879", "#85DCE9", "#F1A8C6", "#AAD99C", "#E1BBF6", "#F3D18C",
}

func aquariumCycle(value, period float64) float64 {
    if period<=0 {return 0}
    return math.Mod(math.Mod(value,period)+period,period)
}

func aquariumTime(m model) float64 {
    return (float64(m.fxSimSteps)+
        float64(m.fxSimAccumulator)/float64(chatFXSimulationStep))/30
}

// aquariumPositions paints only ASCII glyphs on the unused background of
// the visible chat pane. All objects are clipped to the viewport; the normal
// ChatFX renderer keeps text, scrollbar, borders and input completely intact.
func (m model) aquariumPositions(width,height int) []map[int]chatFXCell {
    if width<2 || height<2 {return nil}
    rows:=make([]map[int]chatFXCell,height)
    seconds:=aquariumTime(m)
    salt:=m.fxAquariumSalt
    if salt==0 {salt=m.fxSalt}
    put:=func(x,y int,glyph rune,color string) {
        if x<0 || x>=width || y<0 || y>=height || glyph==' ' {return}
        if rows[y]==nil {rows[y]=make(map[int]chatFXCell)}
        rows[y][x]=chatFXCell{glyph:glyph,color:color}
    }

    // Rooted kelp is generated from the effect's salt, not the frame tick.
    // The stalk bends slightly with the current while remaining attached
    // to the bottom row. The small leaf pattern is intentionally / and \\.
    kelpCount:=min(16,max(2,width/19))
    for i:=0;i<kelpCount;i++ {
        seed:=chatFXHash(salt+uint64(i+1)*0x9e3779b97f4a7c15+0xa91a)
        anchor:=2+int(chatFXUnit(seed+1)*float64(max(1,width-4)))
        length:=3+int(chatFXUnit(seed+2)*float64(max(1,min(height/2,13)-2)))
        if length>height {length=height}
        phase:=chatFXUnit(seed+3)*2*math.Pi
        color:=[]string{"#4EAB70","#6FC78A","#3D9666","#8CCF80"}[int(seed%4)]
        for depth:=0;depth<length;depth++ {
            y:=height-1-depth
            // The rooted base barely moves; higher leaves sway more.
            sway:=math.Sin(seconds*0.53+phase+float64(depth)*0.26)
            x:=anchor+int(math.Round(sway*1.8*float64(depth)/float64(max(1,length-1))))
            glyph:='\\'
            if depth%2==1 {glyph='/'}
            put(x,y,glyph,color)
        }
    }

    // Individual air bubbles start below the viewport and drift upward
    // independently. A few big 'o' rings are accompanied by tiny . , ' :
    // clusters, reminiscent of a quiet aquarium aerator.
    bubbleStreams:=min(15,max(3,width/17))
    for stream:=0;stream<bubbleStreams;stream++ {
        seed:=chatFXHash(salt+uint64(stream+1)*0xbf58476d1ce4e5b9+0xbab1e)
        source:=int(chatFXUnit(seed+1)*float64(width))
        speed:=1.4+2.2*chatFXUnit(seed+2)
        spacing:=3.6+3.2*chatFXUnit(seed+3)
        for slot:=0;slot<6;slot++ {
            offset:=float64(slot)*spacing+chatFXUnit(seed+uint64(slot)+10)*1.5
            travel:=aquariumCycle(seconds*speed+offset,float64(height)+spacing*6)
            y:=height+int(math.Floor(spacing))-int(math.Floor(travel))
            if y<0 || y>=height {continue}
            drift:=math.Sin(seconds*0.7+float64(slot)*1.4+chatFXUnit(seed+4)*6)*2
            x:=source+int(math.Round(drift))
            glyph:='o'
            if (stream+slot)%4!=0 {
                glyph=[]rune{'.',',','\'',':'}[int(chatFXHash(seed+uint64(slot)+20)%4)]
            }
            color:="#80B9CF"
            if glyph=='o' {color="#B5E5F1"}
            put(x,y,glyph,color)
            // Very occasional neighbouring microbubbles, instead of a
            // constant thick column that would compete with chat text.
            if glyph==':' && (stream+slot)%3==0 {
                put(x+1,y-1,'.',"#6A9BAF")
            }
        }
    }

    // Draw the new species after planting kelp and producing air bubbles.
    // Predation, periodic schools and multi-line fish live in their own
    // aquarium-fish module; the same viewport-safe paint function is used.
    m.renderAquariumFish(width,height,seconds,salt,put)
    return rows
}
