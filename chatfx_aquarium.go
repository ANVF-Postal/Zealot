package main

import "math"

// Aquarium is deliberately a lightweight, stateless ChatFX scene. Each
// client uses its usual deterministic salt and 30 Hz animation clock.
// Nothing additional needs to be sent over the room connection.
var aquariumFishColors = [...]string{
    "#F5C879", "#85DCE9", "#F1A8C6", "#AAD99C", "#E1BBF6", "#F3D18C",
}

// Each species has a distinct silhouette and preferred cruising speed.
// Both directions are authored explicitly so eyes and fins face forward.
type aquariumFishSpecies struct {
    left,right string
    speed float64
}

var aquariumSpecies = [...]aquariumFishSpecies{
    {left:"<><", right:"><>", speed:1.0},       // minnow
    {left:"<(o)", right:"(o)>", speed:0.76},    // round fish
    {left:"<|=)", right:"(=|>", speed:0.90},   // striped fish
    {left:"<===", right:"===>", speed:1.45},   // darting needlefish
    {left:"<:))", right:"((:>", speed:0.68},   // plump fish
    {left:"<*)", right:"(*>", speed:1.18},     // little tropical fish
}

func aquariumFishShape(species aquariumFishSpecies,direction int) string {
    if direction<0 {return species.left}
    return species.right
}

// Activity is a finite journey with a quiet interval afterward. All school
// members share a crossing but enter with slightly staggered spacing.
func aquariumSchoolPhase(seconds,initialOffset,travel,rest float64) (float64,bool) {
    phase:=aquariumCycle(seconds+initialOffset,travel+rest)
    return phase,phase<travel
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

    // A few individual fish are always cruising. Their species, speeds and
    // routes differ, so the aquarium doesn't look like a synchronized parade.
    loneCount:=min(9,max(4,width*height/650))
    if height<8 {loneCount=2}
    for i:=0;i<loneCount;i++ {
        seed:=chatFXHash(salt+uint64(i+1)*0x94d049bb133111eb+0xf15a)
        species:=aquariumSpecies[int(chatFXHash(seed+7)%uint64(len(aquariumSpecies)))]
        direction:=1
        if chatFXHash(seed+8)%2==0 {direction=-1}
        shape:=aquariumFishShape(species,direction)
        speed:=(1.1+2.4*chatFXUnit(seed+1))*species.speed
        span:=float64(width+len(shape)+6)
        // Individuals can appear anywhere on the first frame.
        distance:=aquariumCycle(chatFXUnit(seed+2)*span+seconds*speed,span)
        x:=-len(shape)-2+int(math.Floor(distance))
        if direction<0 {x=width+2-int(math.Floor(distance))}
        y:=int(chatFXUnit(seed+3)*float64(height))+
            int(math.Round(math.Sin(seconds*0.55+chatFXUnit(seed+4)*6.28)*0.85))
        y=max(0,min(height-1,y))
        color:=aquariumFishColors[int(chatFXHash(seed+5)%uint64(len(aquariumFishColors)))]
        for j,glyph:=range shape {put(x+j,y,glyph,color)}
    }

    // Occasional schools enter fully from beyond a screen edge, cross
    // together, then leave the aquarium empty of that school for a while.
    // The school is bounded and its schedule comes from the activation salt.
    schoolCount:=min(4,max(1,width/65))
    if height<9 || width<25 {schoolCount=0}
    for school:=0;school<schoolCount;school++ {
        seed:=chatFXHash(salt+uint64(school+1)*0x517cc1b727220a95+0x5c4001)
        species:=aquariumSpecies[int(chatFXHash(seed+2)%uint64(len(aquariumSpecies)))]
        direction:=1
        if chatFXHash(seed+3)%2==0 {direction=-1}
        shape:=aquariumFishShape(species,direction)
        members:=3+int(chatFXHash(seed+4)%3) // 3–5 fish
        speed:=(1.5+1.6*chatFXUnit(seed+5))*species.speed
        spacing:=float64(len(shape)+3)
        // The last fish starts further behind and follows its companions.
        crossing:=float64(width+2*len(shape)+6)+float64(members-1)*spacing
        travel:=crossing/speed
        rest:=14+23*chatFXUnit(seed+6)
        // Let some schools appear immediately, others much later.
        initialOffset:=chatFXUnit(seed+7)*(travel+rest)
        phase,active:=aquariumSchoolPhase(seconds,initialOffset,travel,rest)
        if !active {continue}
        baseY:=2+int(chatFXUnit(seed+8)*float64(max(1,height-4)))
        baseY=min(height-1,baseY)
        color:=aquariumFishColors[int(chatFXHash(seed+9)%uint64(len(aquariumFishColors)))]
        for member:=0;member<members;member++ {
            distance:=phase*speed-float64(member)*spacing
            x:=-len(shape)-2+int(math.Floor(distance))
            if direction<0 {x=width+2-int(math.Floor(distance))}
            y:=baseY+(member%3-1)*2+
                int(math.Round(math.Sin(seconds*0.75+float64(member)*0.6)*0.9))
            y=max(0,min(height-1,y))
            for j,glyph:=range shape {put(x+j,y,glyph,color)}
        }
    }
    return rows
}
