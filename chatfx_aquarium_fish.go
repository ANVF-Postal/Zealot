package main

import (
    "math"
    "strings"
    "unicode/utf8"
)

// Fish are drawn from the user's original artwork. The reversed direction
// mirrors brackets and fins as well as reversing their order.
type aquariumKind uint8
const (
    aquariumPike aquariumKind = iota
    aquariumPerch
    aquariumBluegill
    aquariumLargemouth
    aquariumEel
    aquariumCute
    aquariumWalleye
    aquariumMuskie
)

type aquariumSwimmer struct {
    id uint64
    kind aquariumKind
    x,y int
    vx float64
    direction int
    length int
    mouthOpen bool
    school bool
}

type aquariumChomp struct {
    x,y int
    age float64
}

type aquariumState struct {
    eaten map[uint64]bool // suppress caught fish for its entire crossing
    chomps []aquariumChomp
}

func newAquariumState() *aquariumState {
    return &aquariumState{eaten:make(map[uint64]bool)}
}

func aquariumMirror(line string) string {
    source:=[]rune(line)
    dest:=make([]rune,len(source))
    for i,r:=range source {
        switch r {
        case '<':r='>'
        case '>':r='<'
        case '(':r=')'
        case ')':r='('
        case '{':r='}'
        case '}':r='{'
        case '[':r=']'
        case ']':r='['
        case '/':r='\\'
        case '\\':r='/'
        }
        dest[len(source)-1-i]=r
    }
    return string(dest)
}

var aquariumMuskieClosed=[...]string{
    `|\           |\`,
    `\ \____^_____| \______\  ^`,
    ` |                     --O-\`,
    `/ /===================-----/`,
    `|/`,
}
var aquariumMuskieOpen=[...]string{
    `|\           |\`,
    `\ \____^_____| \______\  V`,
    ` |                     --O-/`,
    `/ /===================-----\`,
    `|/`,
}
// Natural variation in pike and walleye length keeps their elongated forms
// recognisable without substituting unrelated ASCII artwork.
func aquariumFishArt(f aquariumSwimmer) []string {
    n:=max(3,min(10,f.length))
    var art string
    switch f.kind {
    case aquariumPike:art="><"+strings.Repeat("(",n)+"'>"
    case aquariumPerch:
        art="><((<(O-"
        if f.mouthOpen {art="><((<(O<"}
    case aquariumBluegill:art=">((('>"
    case aquariumLargemouth:art="}<((((')"
    case aquariumEel:
        length:=max(9,min(22,f.length))
        art=">"+strings.Repeat("~",length)+"*'>"
    case aquariumCute:art="}<=====Y*>"
    case aquariumWalleye:art="><"+strings.Repeat("(",n)+"*>"
    case aquariumMuskie:
        frame:=aquariumMuskieClosed[:]
        if f.mouthOpen {frame=aquariumMuskieOpen[:]}
        result:=make([]string,len(frame))
        copy(result,frame)
        if f.direction<0 {
            for i:=range result {result[i]=aquariumMirror(result[i])}
        }
        return result
    default:return nil
    }
    if f.direction<0 {art=aquariumMirror(art)}
    return []string{art}
}

func aquariumFishWidth(f aquariumSwimmer) int {
    width:=0
    for _,line:=range aquariumFishArt(f) {
        width=max(width,utf8.RuneCountInString(line))
    }
    return width
}

func aquariumFishHeight(f aquariumSwimmer) int {
    if f.kind==aquariumMuskie {return len(aquariumMuskieClosed)}
    if f.kind==aquariumEel {return 2}
    return 1
}

func aquariumSpeciesColor(kind aquariumKind) string {
    switch kind {
    case aquariumPike:return "#83B977"
    case aquariumPerch:return "#E2C77B"
    case aquariumBluegill:return "#67C399"
    case aquariumLargemouth:return "#7CB78E"
    case aquariumEel:return "#B1B7CE"
    case aquariumCute:return "#E6A3D6"
    case aquariumWalleye:return "#D4BF85"
    case aquariumMuskie:return "#A0B7A2"
    }
    return "#9ACBC0"
}

// The fish renderer adds no opaque background. A bluegill's gill marking
// just behind the eye is blue, and its leading lip is pale green. Pike body
// stripes alternate two greens by position, independently of animation FPS.
func aquariumFishGlyphColor(f aquariumSwimmer,row,index int,glyph rune) string {
    if f.kind==aquariumPike && glyph=='(' || f.kind==aquariumPike && glyph==')' {
        if index%2==0 {return "#A8D990"}
        return "#4B875B"
    }
    if f.kind==aquariumBluegill {
        width:=aquariumFishWidth(f)
        originalIndex:=index
        if f.direction<0 {originalIndex=width-1-index}
        if originalIndex==3 {return "#69A9F7"}
        if glyph=='>' || glyph=='<' {return "#B5F3A5"}
    }
    if f.kind==aquariumWalleye && (glyph=='*' || glyph=='O') {return "#F0F9D4"}
    if f.kind==aquariumMuskie && (glyph=='O' || glyph=='V') {return "#F2E8BD"}
    if f.kind==aquariumPerch && glyph=='O' {return "#FFF1B8"}
    return aquariumSpeciesColor(f.kind)
}

func aquariumFishWeight(seed uint64) aquariumKind {
    value:=chatFXUnit(seed)
    switch {
    case value<0.20:return aquariumPike
    case value<0.42:return aquariumPerch
    case value<0.68:return aquariumBluegill
    case value<0.83:return aquariumLargemouth
    case value<0.93:return aquariumCute
    default:return aquariumWalleye
    }
}

func aquariumFishSpeed(kind aquariumKind,seed uint64) float64 {
    base:=1.0+2.1*chatFXUnit(seed)
    switch kind {
    case aquariumPike:return base*1.28
    case aquariumPerch:return base*1.13
    case aquariumBluegill:return base*0.90
    case aquariumLargemouth:return base*0.78
    case aquariumEel:return base*1.04
    case aquariumCute:return base*(0.78+0.75*chatFXUnit(seed+1))
    case aquariumWalleye:return base*1.52
    case aquariumMuskie:return base*0.85
    }
    return base
}

func aquariumFishDepth(kind aquariumKind,seed uint64,height int) int {
    if height<2 {return 0}
    usable:=max(1,height-2)
    switch kind {
    case aquariumLargemouth:
        return max(0,min(height-1,1+int(chatFXUnit(seed)*float64(max(1,height/4)))))
    case aquariumWalleye:
        return max(0,min(height-2,int(float64(height)*0.69)+int(chatFXUnit(seed)*float64(max(1,height/5)))))
    case aquariumMuskie:
        return max(0,min(height-5,2+int(chatFXUnit(seed)*float64(max(1,height/3)))))
    case aquariumEel:
        return max(0,min(height-3,2+int(chatFXUnit(seed)*float64(max(1,height-6)))))
    default:
        return 1+int(chatFXUnit(seed)*float64(usable))
    }
}

// Solitary fish endlessly cruise, while schools make complete crossings with
// rest windows. Every individual has a stable identity per crossing, allowing
// an eaten fish to disappear without disturbing the other swimmers.
func aquariumSwimmers(width,height int,seconds float64,salt uint64) []aquariumSwimmer {
    if width<8 || height<3 {return nil}
    count:=min(11,max(5,width*height/650))
    if height<8 {count=3}
    swimmers:=make([]aquariumSwimmer,0,count+20)
    for i:=0;i<count;i++ {
        seed:=chatFXHash(salt+uint64(i+1)*0x94d049bb133111eb+0xf15a)
        kind:=aquariumFishWeight(seed+6)
        direction:=1
        if chatFXHash(seed+8)%2==0 {direction=-1}
        length:=3+int(chatFXHash(seed+12)%7)
        speed:=aquariumFishSpeed(kind,seed+1)
        candidate:=aquariumSwimmer{kind:kind,direction:direction,length:length,vx:float64(direction)*speed}
        widthFish:=aquariumFishWidth(candidate)
        span:=float64(width+widthFish+6)
        offset:=chatFXUnit(seed+2)*span
        distance:=aquariumCycle(offset+seconds*speed,span)
        x:=-widthFish-2+int(math.Floor(distance))
        if direction<0 {x=width+2-int(math.Floor(distance))}
        y:=aquariumFishDepth(kind,seed+3,height)
        if kind!=aquariumLargemouth && kind!=aquariumWalleye {
            y+=int(math.Round(math.Sin(seconds*0.58+chatFXUnit(seed+4)*6.28)*0.8))
        }
        y=max(0,min(height-2,y))
        candidate.x,candidate.y=x,y
        // Perch frequently inspect passing fish with an open mouth.
        candidate.mouthOpen=kind==aquariumPerch && aquariumCycle(seconds+chatFXUnit(seed+13)*5,5.2)<1.8
        candidate.id=(uint64(i+1)<<32)|uint64(math.Floor((offset+seconds*speed)/span)+1)
        swimmers=append(swimmers,candidate)
    }

    // Temporary schools: small fish arrive as staggered groups, cross
    // together, then vanish for a quiet spell before the next crossing.
    schoolCount:=min(3,max(1,width/75))
    if height<9 || width<27 {schoolCount=0}
    for school:=0;school<schoolCount;school++ {
        seed:=chatFXHash(salt+uint64(school+1)*0x517cc1b727220a95+0x5c4001)
        species:=[]aquariumKind{aquariumBluegill,aquariumPerch,aquariumCute}[int(chatFXHash(seed+2)%3)]
        direction:=1
        if chatFXHash(seed+3)%2==0 {direction=-1}
        groupSize:=3+int(chatFXHash(seed+4)%3)
        speed:=aquariumFishSpeed(species,seed+5)*1.12
        sample:=aquariumSwimmer{kind:species,direction:direction,length:5}
        fishWidth:=aquariumFishWidth(sample)
        spacing:=float64(fishWidth+3)
        crossing:=float64(width+2*fishWidth+6)+float64(groupSize-1)*spacing
        travel:=crossing/speed
        rest:=17+29*chatFXUnit(seed+6)
        offset:=chatFXUnit(seed+7)*(travel+rest)
        cycle:=math.Floor((seconds+offset)/(travel+rest))
        phase:=aquariumCycle(seconds+offset,travel+rest)
        if phase>=travel {continue}
        baseY:=aquariumFishDepth(species,seed+8,height)
        for member:=0;member<groupSize;member++ {
            distance:=phase*speed-float64(member)*spacing
            x:=-fishWidth-2+int(math.Floor(distance))
            if direction<0 {x=width+2-int(math.Floor(distance))}
            y:=max(0,min(height-2,baseY+(member%3-1)*2))
            candidate:=aquariumSwimmer{
                id:(uint64(100+school*8+member)<<32)|uint64(cycle+1),
                kind:species,direction:direction,length:5,
                x:x,y:y,vx:float64(direction)*speed,school:true,
            }
            candidate.mouthOpen=species==aquariumPerch &&
                aquariumCycle(seconds+float64(member)*0.6,5)<1.4
            swimmers=append(swimmers,candidate)
        }
    }

    // Rare visitors have long inactive periods and complete off-screen
    // crossings. Eels undulate; muskies use the user's full five-line art.
    rare:=[]struct{
        kind aquariumKind
        rest float64
        seed uint64
    }{
        {aquariumEel,90,0xe311},
        {aquariumMuskie,260,0xa115},
    }
    for _,visitor:=range rare {
        if visitor.kind==aquariumMuskie && (width<45 || height<11) {continue}
        if visitor.kind==aquariumEel && (width<30 || height<8) {continue}
        seed:=chatFXHash(salt+visitor.seed)
        kind:=visitor.kind
        direction:=1
        if chatFXHash(seed+3)%2==0 {direction=-1}
        length:=11+int(chatFXHash(seed+4)%11)
        fish:=aquariumSwimmer{kind:kind,direction:direction,length:length}
        fw:=aquariumFishWidth(fish)
        speed:=aquariumFishSpeed(kind,seed+1)
        travel:=float64(width+fw+8)/speed
        // Offset within the inactive period makes appearance unpredictable,
        // with longer pauses for the unusually large muskie.
        offset:=chatFXUnit(seed+2)*visitor.rest
        cycle:=math.Floor((seconds+offset)/(travel+visitor.rest))
        phase:=aquariumCycle(seconds+offset,travel+visitor.rest)
        if phase>=travel {continue}
        distance:=phase*speed
        x:=-fw-4+int(math.Floor(distance))
        if direction<0 {x=width+4-int(math.Floor(distance))}
        fish.id=(uint64(500+kind)<<32)|uint64(cycle+1)
        fish.x,fish.y=x,aquariumFishDepth(kind,seed+5,height)
        fish.vx=float64(direction)*speed
        fish.mouthOpen=kind==aquariumMuskie && aquariumCycle(seconds,3.8)<1.8
        swimmers=append(swimmers,fish)
    }
    return swimmers
}

// Predators overlap the visible bounding boxes of other fish, not their
// kelp/bubbles or protected chat text. Kills are tracked until the prey's
// next crossing and each predation creates one short red *chomp*.
func aquariumCanEat(f aquariumSwimmer) bool {
    return f.kind==aquariumMuskie || f.kind==aquariumWalleye ||
        (f.kind==aquariumPerch && f.mouthOpen)
}

func aquariumSwimmersTouch(a,b aquariumSwimmer) bool {
    aw,bw:=aquariumFishWidth(a),aquariumFishWidth(b)
    ah,bh:=aquariumFishHeight(a),aquariumFishHeight(b)
    return a.x<b.x+bw && b.x<a.x+aw &&
        a.y<b.y+bh && b.y<a.y+ah
}

func (s *aquariumState) advance(m model,width,height int,seconds float64) {
    if s==nil {return}
    // Expire tombstones only when that crossing's identity disappears;
    // a swallowed fish must never pop back into the same journey.
    live:=s.chomps[:0]
    for _,c:=range s.chomps {
        c.age+=bubbleStepSeconds
        if c.age<0.65 {live=append(live,c)}
    }
    s.chomps=live
    salt:=m.fxAquariumSalt
    if salt==0 {salt=m.fxSalt}
    fish:=aquariumSwimmers(width,height,seconds,salt)
    current:=make(map[uint64]bool,len(fish))
    for _,swimmer:=range fish {current[swimmer.id]=true}
    for id:=range s.eaten {if !current[id] {delete(s.eaten,id)}}
    for _,hunter:=range fish {
        if !aquariumCanEat(hunter) || s.eaten[hunter.id] {continue}
        for _,prey:=range fish {
            if prey.id==hunter.id || prey.kind==aquariumMuskie ||
                s.eaten[prey.id] {continue}
            }
            if !aquariumSwimmersTouch(hunter,prey) {continue}
            // A caught fish stays away long enough for the predator to pass;
            // it reappears on its next independent crossing.
            s.eaten[prey.id]=true
            if len(s.chomps)<16 {
                s.chomps=append(s.chomps,aquariumChomp{
                    x:max(0,min(width-1,prey.x)),y:max(0,prey.y-1),
                })
            }
        }
    }
}

func (m model) renderAquariumFish(width,height int,seconds float64,salt uint64,put func(int,int,rune,string)) {
    for _,fish:=range aquariumSwimmers(width,height,seconds,salt) {
        if m.fxAquarium!=nil && m.fxAquarium.eaten[fish.id] {continue}
        lines:=aquariumFishArt(fish)
        if fish.kind==aquariumEel && len(lines)>0 {
            // The eel bends mid-body when changing depth, trailing its tail
            // into a second row rather than rigidly sliding vertically.
            bend:=int(math.Round(math.Sin(seconds*0.9+float64(fish.id%17))))
            if bend!=0 && fish.length>=11 {
                line:=[]rune(lines[0])
                kink:=len(line)*2/3
                if kink<len(line)-2 {
                    for i,glyph:=range line {
                        localY:=0
                        if i>kink {localY=bend}
                        if i==kink {glyph='\\';if bend<0 {glyph='/'}}
                        put(fish.x+i,fish.y+localY,glyph,aquariumFishGlyphColor(fish,0,i,glyph))
                    }
                    continue
                }
            }
        }
        for row,line:=range lines {
            for col,glyph:=range line {
                put(fish.x+col,fish.y+row,glyph,aquariumFishGlyphColor(fish,row,col,glyph))
            }
        }
    }
    if m.fxAquarium!=nil {
        for _,chomp:=range m.fxAquarium.chomps {
            if chomp.age>0.48 {continue}
            text:="*chomp*"
            for i,glyph:=range text {
                put(chomp.x+i-3,chomp.y,glyph,"#F35B62")
            }
        }
    }
}
