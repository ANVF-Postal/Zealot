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
    aquariumMinnow
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
    seed uint64 // individual motion and eel body follow-through
    route aquariumRoute
    routePhase float64
    baseY int
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
    case aquariumMinnow:art="><>"
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
            // Mirror across the entire multi-line sprite width, not each
            // row's own width; short fin rows stay attached to the body.
            fullWidth:=0
            for _,line:=range result {fullWidth=max(fullWidth,utf8.RuneCountInString(line))}
            for i,line:=range result {
                result[i]=aquariumMirror(line+strings.Repeat(" ",fullWidth-utf8.RuneCountInString(line)))
            }
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
    case aquariumMinnow:return "#96DAD6"
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
func aquariumMinnowColor(seed uint64) string {
    // Stable per-fish shades, without animated flicker or extreme brightness.
    palette:=[...]string{
        "#96DAD6", // seafoam
        "#86C6D5", // soft blue
        "#B6DFCA", // pale mint
        "#85CAB3", // green-blue
        "#C2D2E9", // lavender-blue
        "#A5D3B4", // gentle green
        "#B6DAE6", // powder blue
        "#C3DFD1", // silver mint
    }
    return palette[int(chatFXHash(seed+0x6d696e6e6f77)%uint64(len(palette)))]
}

func aquariumFishGlyphColor(f aquariumSwimmer,row,index int,glyph rune) string {
    if f.kind==aquariumMinnow {return aquariumMinnowColor(f.seed)}
    originalIndex:=index
    if f.direction<0 {originalIndex=aquariumFishWidth(f)-1-index}
    if f.kind==aquariumPike && (glyph=='(' || glyph==')') {
        // The bands belong to body segments, not the animation clock.
        if (originalIndex-2)%2==0 {return "#94C18A"}
        return "#649466"
    }
    if f.kind==aquariumPerch && originalIndex>=2 && originalIndex<=5 &&
        (glyph=='(' || glyph==')' || glyph=='<') {
        if (originalIndex-2)%2==0 {return "#E6AA61"}
        return "#AF702E"
    }
    if f.kind==aquariumBluegill {
        if originalIndex==3 {return "#69A9F7"}
        if glyph=='>' || glyph=='<' {return "#B5F3A5"}
    }
    if f.kind==aquariumLargemouth && originalIndex==4 {
        return "#598969" // second body segment behind the eye
    }
    if f.kind==aquariumWalleye && (glyph=='*' || glyph=='O') {return "#F0F9D4"}
    if f.kind==aquariumMuskie && (glyph=='O' || glyph=='V') {return "#F2E8BD"}
    if f.kind==aquariumPerch && glyph=='O' {return "#FFF1B8"}
    return aquariumSpeciesColor(f.kind)
}

func aquariumFishSpeed(kind aquariumKind,seed uint64) float64 {
    base:=1.0+2.1*chatFXUnit(seed)
    switch kind {
    case aquariumMinnow:return base*1.4
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

// Every fish has a self-contained journey: enter, roam, turn at one or two
// personal waypoints, leave the screen and spend time away. Crucially this
// path is continuous; the sprite never teleports when it changes heading.
type aquariumRoute struct {
    points [4]float64
    count int
    speed float64
    pause float64
    travel float64
}

func aquariumMakeRoute(width,fishWidth int,initialDirection int,speed float64,seed uint64,turns int) aquariumRoute {
    r:=aquariumRoute{speed:speed,pause:0.4+1.6*chatFXUnit(seed+21)}
    if initialDirection>0 {
        r.points=[4]float64{-float64(fishWidth+3),
            float64(width)*(0.60+0.22*chatFXUnit(seed+22)),
            float64(width)*(0.18+0.22*chatFXUnit(seed+23)),
            float64(width+3)}
    } else {
        r.points=[4]float64{float64(width+3),
            float64(width)*(0.18+0.22*chatFXUnit(seed+22)),
            float64(width)*(0.60+0.22*chatFXUnit(seed+23)),
            -float64(fishWidth+3)}
    }
    if turns<1 {turns=1}
    if turns>2 {turns=2}
    r.count=turns+2
    if turns==1 {r.points[2]=r.points[0]}
    for i:=0;i<r.count-1;i++ {
        r.travel+=math.Abs(r.points[i+1]-r.points[i])/speed
        if i<r.count-2 {r.travel+=r.pause}
    }
    return r
}

func aquariumRouteAt(r aquariumRoute,phase float64) (float64,int,bool) {
    if r.count<2 || r.speed<=0 || phase<0 || phase>=r.travel {return 0,1,false}
    for i:=0;i<r.count-1;i++ {
        from,to:=r.points[i],r.points[i+1]
        duration:=math.Abs(to-from)/r.speed
        dir:=1
        if to<from {dir=-1}
        if phase<duration {
            return from+float64(dir)*phase*r.speed,dir,true
        }
        phase-=duration
        if i<r.count-2 {
            if phase<r.pause {return to,-dir,true} // fish pauses before turning
            phase-=r.pause
        }
    }
    return r.points[r.count-1],1,false
}

func aquariumOtherSpecies(seed uint64) aquariumKind {
    value:=chatFXUnit(seed)
    switch {
    case value<0.14:return aquariumPike
    case value<0.31:return aquariumPerch
    case value<0.55:return aquariumBluegill
    case value<0.74:return aquariumLargemouth
    case value<0.85:return aquariumCute
    default:return aquariumWalleye
    }
}

func aquariumFishRest(kind aquariumKind,seed uint64) float64 {
    switch kind {
    case aquariumMinnow:return 3+8*chatFXUnit(seed+33)
    case aquariumPerch:return 22+31*chatFXUnit(seed+33)
    case aquariumWalleye:return 24+32*chatFXUnit(seed+33)
    case aquariumEel:return 220+80*chatFXUnit(seed+33)
    case aquariumMuskie:return 1150+300*chatFXUnit(seed+33)
    default:return 12+25*chatFXUnit(seed+33)
    }
}

func aquariumFishY(f aquariumSwimmer,seconds float64,height int) int {
    if f.kind==aquariumMuskie {return max(0,min(height-5,f.baseY))}
    base:=f.baseY
    seed:=f.seed
    switch f.kind {
    case aquariumMinnow:
        base+=int(math.Round(1.4*math.Sin(seconds*0.95+chatFXUnit(seed+1)*6.283)))
    case aquariumPike:
        base+=int(math.Round(0.65*math.Sin(seconds*0.21+chatFXUnit(seed+2)*6.283)))
    case aquariumPerch:
        base+=int(math.Round(0.8*math.Sin(seconds*0.48+chatFXUnit(seed+3)*6.283)))
    case aquariumBluegill:
        base+=int(math.Round(1.3*math.Sin(seconds*0.37+chatFXUnit(seed+4)*6.283)))
    case aquariumLargemouth:
        base+=int(math.Round(0.5*math.Sin(seconds*0.26+chatFXUnit(seed+5)*6.283)))
    case aquariumWalleye:
        base+=int(math.Round(0.7*math.Sin(seconds*0.19+chatFXUnit(seed+6)*6.283)))
    case aquariumCute:
        base+=int(math.Round(2.4*math.Sin(seconds*0.83+chatFXUnit(seed+7)*6.283)))
    case aquariumEel:
        return aquariumEelHeadY(f,seconds,height)
    }
    switch f.kind {
    case aquariumLargemouth:
        return max(0,min(max(1,height/4),base))
    case aquariumWalleye:
        return max(height*2/3,min(height-2,base))
    default:
        return max(0,min(height-2,base))
    }
}

// The number of minnow slots scales with the viewport, and schools contain
// minnows almost exclusively. Large predators are deliberately infrequent.
func aquariumSwimmers(width,height int,seconds float64,salt uint64) []aquariumSwimmer {
    if width<8 || height<3 {return nil}
    minnowCount:=min(46,max(14,width*height/230))
    otherCount:=min(12,max(4,width*height/850))
    if height<8 {minnowCount=5;otherCount=1}
    swimmers:=make([]aquariumSwimmer,0,minnowCount+otherCount+32)

    add:=func(idBase uint64,kind aquariumKind,seed uint64,school bool,member int) {
        direction:=1
        if chatFXHash(seed+8)%2==0 {direction=-1}
        length:=3+int(chatFXHash(seed+12)%7)
        if kind==aquariumEel {length=11+int(chatFXHash(seed+12)%11)}
        speed:=aquariumFishSpeed(kind,seed+1)
        if kind==aquariumEel {speed*=3.2} // >2x the previous eel speed
        if kind==aquariumMuskie {speed*=3.2}
        if school {speed*=1.2}
        candidate:=aquariumSwimmer{
            kind:kind,direction:direction,length:length,
            seed:seed,vx:float64(direction)*speed,school:school,
            baseY:aquariumFishDepth(kind,seed+3,height),
        }
        fw:=aquariumFishWidth(candidate)
        if idBase>=100 && idBase<200 {fw=max(14,fw)}
        turns:=1+int(chatFXHash(seed+17)%2)
        if kind==aquariumMuskie {turns=1} // a deliberate sweep and departure
        route:=aquariumMakeRoute(width,fw,direction,speed,seed,turns)
        rest:=aquariumFishRest(kind,seed)
        if school {rest+=route.travel*0.95+12}
        period:=route.travel+rest
        initialOffset:=chatFXUnit(seed+16)*period
        if school {initialOffset-=float64(member)*float64(fw+3)/speed}
        clock:=seconds+initialOffset
        cycle:=math.Floor(clock/period)
        phase:=aquariumCycle(clock,period)
        x,heading,active:=aquariumRouteAt(route,phase)
        if !active {return}
        if idBase>=100 && idBase<200 {
            // A new individual replaces the last visitor after departure.
            // Species, markings and depth only change while offscreen.
            generation:=uint64(int64(cycle)+65537)
            lifeSeed:=chatFXHash(seed+generation*0x9e3779b97f4a7c15)
            candidate.kind=aquariumOtherSpecies(lifeSeed)
            candidate.length=3+int(chatFXHash(lifeSeed+12)%7)
            candidate.baseY=aquariumFishDepth(candidate.kind,lifeSeed+3,height)
        }
        candidate.x=int(math.Floor(x))
        candidate.direction=heading
        candidate.vx=float64(heading)*speed
        candidate.id=(idBase<<32)|uint64(int64(cycle)+1+65536)
        candidate.mouthOpen=kind==aquariumPerch &&
            aquariumCycle(seconds+chatFXUnit(seed+13)*6,5.4)<1.4
        if kind==aquariumMuskie {
            candidate.mouthOpen=aquariumCycle(seconds+chatFXUnit(seed+24)*3,3.8)<1.8
        }
        candidate.y=aquariumFishY(candidate,seconds,height)
        candidate.route=route
        candidate.routePhase=phase
        swimmers=append(swimmers,candidate)
    }

    for i:=0;i<minnowCount;i++ {
        seed:=chatFXHash(salt+uint64(i+1)*0x94d049bb133111eb+0xf15a)
        add(uint64(i+1),aquariumMinnow,seed,false,0)
    }
    for i:=0;i<otherCount;i++ {
        seed:=chatFXHash(salt+uint64(i+1)*0x5274a122ab9038bd+0x2917)
        add(uint64(100+i),aquariumOtherSpecies(seed+6),seed,false,0)
    }

    // School visits are shared journeys, not permanent groups. A common
    // seed keeps their heading and route aligned, with staggered arrivals.
    schoolCount:=min(3,max(1,width/75))
    if height<9 || width<27 {schoolCount=0}
    for school:=0;school<schoolCount;school++ {
        seed:=chatFXHash(salt+uint64(school+1)*0x517cc1b727220a95+0x5c4001)
        members:=5+int(chatFXHash(seed+4)%5)
        schoolKind:=aquariumMinnow
        if chatFXUnit(seed+18)>0.92 {schoolKind=aquariumBluegill}
        for member:=0;member<members;member++ {
            // Distinct identities, staggered timing, shared depth and course.
            add(uint64(200+school*16+member),schoolKind,seed,true,member)
            if len(swimmers)>0 {
                f:=&swimmers[len(swimmers)-1]
                if f.school && f.id>>32==uint64(200+school*16+member) {
                    f.y=max(0,min(height-2,f.y+(member%3-1)*2))
                }
            }
        }
    }

    // Eels and muskies are rare enough to be an event, not tank residents.
    if width>=30 && height>=8 {
        seed:=chatFXHash(salt+0xe311)
        add(500,aquariumEel,seed,false,0)
    }
    if width>=45 && height>=11 {
        seed:=chatFXHash(salt+0xa115)
        add(501,aquariumMuskie,seed,false,0)
    }
    aquariumMinnowAvoidance(swimmers,height)
    return swimmers
}

// Minnows notice nearby mouths and steer vertically out of the way.
// The small distance-weighted movement doesn't teleport them to a new lane;
// it is recalculated smoothly as a hunter approaches or recedes.
func aquariumMinnowAvoidance(fish []aquariumSwimmer,height int) {
    for i:=range fish {
        if fish[i].kind!=aquariumMinnow {continue}
        shift:=0.0
        for j:=range fish {
            if !aquariumCanEat(fish[j]) {continue}
            dx:=math.Abs(float64(fish[i].x-fish[j].x))
            dy:=math.Abs(float64(fish[i].y-fish[j].y))
            if dx>17 || dy>5 {continue}
            strength:=(1-dx/17)*(1-dy/5)
            if fish[i].y<=fish[j].y {shift-=3*strength} else {shift+=3*strength}
        }
        fish[i].y=max(0,min(height-2,fish[i].y+int(math.Round(shift))))
    }
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
    if a.x>=b.x+bw || b.x>=a.x+aw ||
        a.y>=b.y+bh || b.y>=a.y+ah {return false}
    // Most species are solid single-row sprites, so their overlapping
    // bounding boxes already represent a real touch.
    if a.kind!=aquariumMuskie && b.kind!=aquariumMuskie {return true}
    // A muskie has large gaps between its fins. Check printed ASCII
    // cells, so it cannot swallow a fish through empty visual space.
    left:=max(a.x,b.x)
    right:=min(a.x+aw,b.x+bw)
    top:=max(a.y,b.y)
    bottom:=min(a.y+ah,b.y+bh)
    artA,artB:=aquariumFishArt(a),aquariumFishArt(b)
    for y:=top;y<bottom;y++ {
        rowA,rowB:=[]rune(artA[y-a.y]),[]rune(artB[y-b.y])
        for x:=left;x<right;x++ {
            ax,bx:=x-a.x,x-b.x
            if ax<len(rowA) && bx<len(rowB) &&
                rowA[ax]!=' ' && rowB[bx]!=' ' {return true}
        }
    }
    return false
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
    s.resolvePredation(fish,width)

}

// Resolve visible overlapping predator/prey pairs once per fixed tick.
func (s *aquariumState) resolvePredation(fish []aquariumSwimmer,width int) {
    current:=make(map[uint64]bool,len(fish))
    for _,swimmer:=range fish {current[swimmer.id]=true}
    for id:=range s.eaten {if !current[id] {delete(s.eaten,id)}}
    for _,hunter:=range fish {
        if !aquariumCanEat(hunter) || s.eaten[hunter.id] ||
            hunter.x>=width || hunter.x+aquariumFishWidth(hunter)<=0 {continue}
        for _,prey:=range fish {
            if prey.id==hunter.id || prey.kind==aquariumMuskie ||
                s.eaten[prey.id] || prey.x>=width ||
                prey.x+aquariumFishWidth(prey)<=0 {continue}
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

// A body point follows the head's path from a slightly earlier moment.
// Successive ~ segments follow the same route at increasing delays: during a
// climb they trace a gentle staircase instead of snapping into a rigid kink.
type aquariumEelPoint struct {
    x,y int
    glyph rune
}

func aquariumEelHeadY(f aquariumSwimmer,seconds float64,height int) int {
    phase:=chatFXUnit(f.seed+8)*2*math.Pi
    depth:=float64(f.baseY)+3.0*math.Sin(seconds*0.28+phase)+
        0.75*math.Sin(seconds*0.51+chatFXUnit(f.seed+9)*2*math.Pi)
    return max(1,min(height-2,int(math.Round(depth))))
}

// The terminal head of the eel is three glyphs, "*'>", not a single
// character. All three occupy the current head row, while only the "~"
// body and the leading tail marker sample the head's earlier path.
func aquariumEelBody(f aquariumSwimmer,seconds float64,height int) []aquariumEelPoint {
    art:=aquariumFishArt(f)
    if len(art)==0 {return nil}
    runes:=[]rune(art[0])
    points:=make([]aquariumEelPoint,0,len(runes))
    n:=len(runes)
    if n<4 {return nil}
    headStart,headEnd:=n-3,n-1
    if f.direction<0 {headStart,headEnd=0,2}
    headY:=aquariumEelHeadY(f,seconds,height)
    headX:=float64(f.x)
    // route position is always the left edge of the fish's bounding box.
    // A head moving right sits on the rightmost three columns; a head
    // moving left occupies the leftmost three.
    for index,glyph:=range runes {
        if index>=headStart && index<=headEnd {
            points=append(points,aquariumEelPoint{
                x:int(headX)+index,y:headY,glyph:glyph,
            })
            continue
        }
        gap:=0
        tailOffset:=0
        if f.direction>0 {
            gap=headStart-index
            tailOffset=headStart
        } else {
            gap=index-headEnd
            tailOffset=headEnd
        }
        // One column of the trailing body corresponds to one speed unit of
        // historical travel. The tail reproduces climbs and descents from
        // the exact same path rather than using an independent wiggle.
        lag:=float64(gap)/math.Max(1,f.route.speed)
        pastX,_,ok:=aquariumRouteAt(f.route,f.routePhase-lag)
        if !ok {continue} // body appears gradually as the eel enters
        x:=int(math.Round(pastX))+tailOffset
        points=append(points,aquariumEelPoint{
            x:x,y:aquariumEelHeadY(f,seconds-lag,height),glyph:glyph,
        })
    }
    return points
}

func aquariumPaintEel(f aquariumSwimmer,seconds float64,height int,put func(int,int,rune,string)) {
    points:=aquariumEelBody(f,seconds,height)
    // Draw from tail toward head so overlapping segments during a U-turn
    // never erase the eye and mouth.
    headIndex:=len(points)-1
    if f.direction<0 {headIndex=0}
    previous:=aquariumEelPoint{}
    havePrevious:=false
    for step:=0;step<len(points);step++ {
        index:=step
        if headIndex==0 {index=len(points)-1-step}
        point:=points[index]
        if havePrevious {
            dx:=point.x-previous.x
            dy:=point.y-previous.y
            if math.Abs(float64(dx))<=2 && math.Abs(float64(dy))<=2 &&
                (dx!=0 || dy!=0) {
                // Connect rising/falling segments only where neighbouring
                // dots would otherwise have an obvious one-cell gap.
                if dy!=0 {
                    connector:='/'
                    if dx*dy>0 {connector='\\'}
                    put(previous.x,previous.y,connector,aquariumSpeciesColor(aquariumEel))
                }
            }
        }
        put(point.x,point.y,point.glyph,aquariumSpeciesColor(aquariumEel))
        previous=point
        havePrevious=true
    }
}

func (m model) renderAquariumFish(width,height int,seconds float64,salt uint64,put func(int,int,rune,string)) {
    for _,fish:=range aquariumSwimmers(width,height,seconds,salt) {
        if m.fxAquarium!=nil && m.fxAquarium.eaten[fish.id] {continue}
        lines:=aquariumFishArt(fish)
        if fish.kind==aquariumEel {
            aquariumPaintEel(fish,seconds,height,put)
            continue
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
