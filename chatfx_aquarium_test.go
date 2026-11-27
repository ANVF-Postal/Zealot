package main

import (
    "bytes"
    "encoding/json"
    "reflect"
    "strings"
    "testing"
)

func TestAquariumViewportClippingAndFishAndKelp(t *testing.T) {
    for _,size:=range [][2]int{{2,2},{10,5},{24,10},{80,25},{160,45},{240,70}} {
        m:=newMessageInteractionTestModel(size[0]+2)
        m.fxRoom=chatFXState{name:"aquarium"}
        m.fxSalt=73
        rows:=m.aquariumPositions(size[0],size[1])
        if len(rows)!=size[1] {t.Fatalf("%v: got %d rows",size,len(rows))}
        fish,green,bubbles:=0,0,0
        for y,row:=range rows {
            for x,cell:=range row {
                if x<0 || x>=size[0] || y<0 || y>=size[1] {
                    t.Fatalf("%v: aquarium escaped at (%d,%d)",size,x,y)
                }
                if cell.glyph=='<' || cell.glyph=='>' {fish++}
                if cell.glyph=='/' || cell.glyph=='\\' {green++}
                if strings.ContainsRune("o.,':",cell.glyph) {bubbles++}
                if cell.background!="" {t.Fatal("aquarium must not paint over chat backgrounds")}
            }
        }
        if size[0]>=80 && (fish==0 || green==0 || bubbles==0) {
            t.Fatalf("%v: fish=%d kelp=%d bubbles=%d",size,fish,green,bubbles)
        }
    }
}

func TestAquariumUsesDeterministic30HzClock(t *testing.T) {
    m:=newMessageInteractionTestModel(130)
    m.fxRoom=chatFXState{name:"aquarium"}
    m.fxSalt=71
    m.fxSimSteps=150
    first:=m.aquariumPositions(128,30)
    for _,fps:=range []int{1,15,30,60} {
        m.fxRenderFPS=fps
        m.fxFrame=uint64(fps*100)
        if got:=m.aquariumPositions(128,30);!reflect.DeepEqual(first,got) {
            t.Fatalf("aquarium depended on render FPS %d instead of simulation clock",fps)
        }
    }
    m.fxSimSteps+=270
    if reflect.DeepEqual(first,m.aquariumPositions(128,30)) {
        t.Fatal("fish, kelp and rising bubbles never animated")
    }
}

func TestAquariumSpeciesUseRequestedASCII(t *testing.T) {
    samples:=[]struct{kind aquariumKind;length int;closed,open string}{
        {aquariumPike,5,"><((((('>",""},
        {aquariumPerch,0,"><((<(O-","><((<(O<"},
        {aquariumBluegill,0,">((('>",""},
        {aquariumLargemouth,0,"}<((((')",""},
        {aquariumEel,15,">~~~~~~~~~~~~~~~*'>",""},
        {aquariumCute,0,"}<=====Y*>",""},
        {aquariumWalleye,5,"><(((((*>",""},
    }
    for _,tc:=range samples {
        f:=aquariumSwimmer{kind:tc.kind,length:tc.length,direction:1}
        art:=aquariumFishArt(f)
        if len(art)!=1 || art[0]!=tc.closed {
            t.Fatalf("species %d: got %q, want %q",tc.kind,art,tc.closed)
        }
        if tc.open!="" {
            f.mouthOpen=true
            if open:=aquariumFishArt(f)[0];open!=tc.open {
                t.Fatalf("species %d open mouth: got %q, want %q",tc.kind,open,tc.open)
            }
        }
        f.direction=-1
        f.mouthOpen=false
        if reversed:=aquariumFishArt(f)[0];reversed!=aquariumMirror(art[0]) {
            t.Fatalf("species %d closed-mouth pose wasn't mirrored correctly: got %q, want %q",tc.kind,reversed,aquariumMirror(art[0]))
        }
        if tc.open!="" {
            f.mouthOpen=true
            if reversed:=aquariumFishArt(f)[0];reversed!=aquariumMirror(tc.open) {
                t.Fatalf("species %d open-mouth pose wasn't mirrored correctly: got %q, want %q",tc.kind,reversed,aquariumMirror(tc.open))
            }
        }
    }
    for _,mouth:=range []bool{false,true} {
        f:=aquariumSwimmer{kind:aquariumMuskie,direction:1,mouthOpen:mouth}
        lines:=aquariumFishArt(f)
        if len(lines)!=5 {t.Fatalf("muskie lost its five-line art: %q",lines)}
        if !strings.Contains(lines[2],"--O-") {t.Fatal("muskie lost its eye")}
        if mouth && !strings.Contains(lines[1]," V") {t.Fatal("open muskie pose missing")}
        if !mouth && !strings.Contains(lines[1]," ^") {t.Fatal("closed muskie pose missing")}
        f.direction=-1
        mirrored:=aquariumFishArt(f)
        if len(mirrored[0])!=len(mirrored[4]) || len(mirrored[1])!=len(mirrored[4]) {
            t.Fatal("left-facing muskie fin rows weren't padded to body width")
        }
        if strings.TrimSpace(mirrored[4])=="" {
            t.Fatal("left-facing muskie lost its lower tail fin")
        }
    }
}

func TestAquariumSpeciesBehavioursAndColour(t *testing.T) {
    for i:=0;i<128;i++ {
        seed:=uint64(i)
        if y:=aquariumFishDepth(aquariumLargemouth,seed,40);y>12 {
            t.Fatalf("largemouth left the surface region: %d",y)
        }
        if y:=aquariumFishDepth(aquariumWalleye,seed,40);y<27 {
            t.Fatalf("walleye left the lower region: %d",y)
        }
    }
    bluegill:=aquariumSwimmer{kind:aquariumBluegill,length:3,direction:1}
    if aquariumFishGlyphColor(bluegill,0,3,'(')!="#69A9F7" {
        t.Fatal("bluegill blue gill marking missing")
    }
    if aquariumFishGlyphColor(bluegill,0,0,'>')!="#B5F3A5" {
        t.Fatal("bluegill lip not highlighted")
    }
    pike:=aquariumSwimmer{kind:aquariumPike,length:5,direction:1}
    if aquariumFishGlyphColor(pike,0,2,'(')==aquariumFishGlyphColor(pike,0,3,'(') {
        t.Fatal("pike body stripes don't alternate greens")
    }
    if aquariumFishSpeed(aquariumWalleye,23)<=aquariumFishSpeed(aquariumLargemouth,23) {
        t.Fatal("walleye should cruise faster than largemouth")
    }
}

func TestAquariumPredationAndChomp(t *testing.T) {
    prey:=aquariumSwimmer{id:1,kind:aquariumBluegill,direction:1,x:22,y:9,length:3}
    perch:=aquariumSwimmer{id:2,kind:aquariumPerch,direction:1,x:20,y:9,mouthOpen:false}
    state:=newAquariumState()
    state.resolvePredation([]aquariumSwimmer{prey,perch},100)
    if state.eaten[prey.id] {t.Fatal("closed-mouth perch ate another fish")}
    perch.mouthOpen=true
    state.resolvePredation([]aquariumSwimmer{prey,perch},100)
    if !state.eaten[prey.id] || len(state.chomps)!=1 {
        t.Fatal("open-mouth perch did not eat prey with a chomp")
    }
    state.resolvePredation([]aquariumSwimmer{prey,perch},100)
    if len(state.chomps)!=1 {t.Fatal("eaten fish was chomped twice")}
    state.resolvePredation([]aquariumSwimmer{perch},100)
    if state.eaten[prey.id] {t.Fatal("prey's tombstone outlived its crossing")}
    for _,hunter:=range []aquariumKind{aquariumWalleye,aquariumMuskie} {
        s:=newAquariumState()
        big:=aquariumSwimmer{id:3,kind:hunter,direction:1,x:20,y:9,length:6}
        if hunter==aquariumMuskie {big.y=8} // its multi-row body reaches the prey
        s.resolvePredation([]aquariumSwimmer{big,prey},100)
        if !s.eaten[prey.id] {t.Fatalf("predator %d failed to eat prey",hunter)}
    }
    m:=newMessageInteractionTestModel(100)
    m.fxAquarium=state
    state.chomps=[]aquariumChomp{{x:30,y:7}}
    rendered:=map[int]rune{}
    m.renderAquariumFish(98,24,0,42,func(x,y int,glyph rune,color string) {
        if y==7 {rendered[x]=glyph}
    })
    text:=""
    for x:=27;x<=33;x++ {text+=string(rendered[x])}
    if text!="*chomp*" {t.Fatalf("red chomp text missing: %q",text)}
}

func TestAquariumMuskieOnlyChompsOnPrintedContact(t *testing.T) {
    muskie:=aquariumSwimmer{id:10,kind:aquariumMuskie,direction:1,x:20,y:8}
    prey:=aquariumSwimmer{id:11,kind:aquariumBluegill,direction:1,x:24,y:8}
    if aquariumSwimmersTouch(muskie,prey) {
        t.Fatal("muskie ate a fish floating through a blank gap in its upper art")
    }
    prey.x,prey.y=20,9
    if !aquariumSwimmersTouch(muskie,prey) {
        t.Fatal("muskie failed to register actual ASCII contact")
    }
}

func TestAquariumRareVisitorsHaveLongAbsences(t *testing.T) {
    eel,muskie:=0,0
    for sec:=0;sec<1800;sec+=4 {
        fish:=aquariumSwimmers(160,35,float64(sec),4242)
        for _,f:=range fish {
            if f.x>=160 || f.x+aquariumFishWidth(f)<=0 {continue}
            if f.kind==aquariumEel {eel++}
            if f.kind==aquariumMuskie {muskie++}
        }
    }
    if eel==0 || muskie==0 {t.Fatalf("rare visitors never arrived: eel %d muskie %d",eel,muskie)}
    if muskie>=eel || muskie>35 {
        t.Fatalf("muskie should be extremely rare: eel %d muskie %d",eel,muskie)
    }
}

func TestAquariumFishBandsStayFixedWhileSwimmingAndTurning(t *testing.T) {
    short:=aquariumSwimmer{kind:aquariumPike,direction:1,length:3}
    long:=aquariumSwimmer{kind:aquariumPike,direction:1,length:9}
    if aquariumFishWidth(long)<=aquariumFishWidth(short) {t.Fatal("pike length didn't vary")}
    first:=aquariumFishGlyphColor(short,0,2,'(')
    if aquariumFishGlyphColor(short,0,2,'(')!=first {
        t.Fatal("pike stripe colours changed for an unchanged body")
    }
    if aquariumFishGlyphColor(short,0,3,'(')==first {
        t.Fatal("pike body lacks alternating greens")
    }
    short.direction=-1
    if aquariumFishGlyphColor(short,0,aquariumFishWidth(short)-1-2,')')!=first {
        t.Fatal("pike stripe pattern reversed incorrectly with the fish")
    }
    perch:=aquariumSwimmer{kind:aquariumPerch,direction:1}
    if aquariumFishGlyphColor(perch,0,2,'(')==aquariumFishGlyphColor(perch,0,3,'(') {
        t.Fatal("perch orange bands aren't alternating")
    }
    if aquariumFishGlyphColor(perch,0,2,'(')!="#E6AA61" {
        t.Fatal("perch band colour should stay a permanent orange")
    }
    perch.direction=-1
    if aquariumFishGlyphColor(perch,0,aquariumFishWidth(perch)-1-2,')' )!="#E6AA61" {
        t.Fatal("perch stripes must remain fixed to the body after turning")
    }
    bass:=aquariumSwimmer{kind:aquariumLargemouth,direction:1}
    if aquariumFishGlyphColor(bass,0,4,'(')==aquariumSpeciesColor(aquariumLargemouth) {
        t.Fatal("largemouth second body segment wasn't darkened")
    }
    a:=aquariumSwimmer{kind:aquariumWalleye,direction:1,length:3}
    b:=aquariumSwimmer{kind:aquariumWalleye,direction:1,length:10}
    if aquariumFishWidth(b)<=aquariumFishWidth(a) {t.Fatal("walleye length didn't vary")}
}

func TestAquariumMinnowsAreMajority(t *testing.T) {
    for _,dims:=range [][2]int{{80,25},{128,32},{180,50}} {
        minnows,others:=0,0
        for sec:=0;sec<90;sec+=3 {
            for _,f:=range aquariumSwimmers(dims[0],dims[1],float64(sec),1234) {
                if f.x>=dims[0] || f.x+aquariumFishWidth(f)<=0 {continue}
                if f.kind==aquariumMinnow {minnows++} else {others++}
            }
        }
        if minnows<others*2 {
            t.Fatalf("%v: minnows must dominate, got %d minnows vs %d other fish",dims,minnows,others)
        }
    }
    if art:=aquariumFishArt(aquariumSwimmer{kind:aquariumMinnow,direction:-1});len(art)!=1 || art[0]!="<><" {
        t.Fatalf("minnow art is wrong: %q",art)
    }
}

func TestAquariumFishChangeDirectionAndEventuallyLeave(t *testing.T) {
    for _,kind:=range []aquariumKind{
        aquariumMinnow,aquariumPike,aquariumPerch,aquariumBluegill,
        aquariumLargemouth,aquariumCute,aquariumWalleye,aquariumEel,aquariumMuskie,
    } {
        seed:=chatFXHash(uint64(kind)+7123)
        f:=aquariumSwimmer{kind:kind,length:6,direction:1}
        width:=aquariumFishWidth(f)
        speed:=aquariumFishSpeed(kind,seed+1)
        route:=aquariumMakeRoute(120,width,1,speed,seed,2)
        previousDirection:=0
        turnCount:=0
        activeCount:=0
        for i:=0;i<1200;i++ {
            phase:=route.travel*float64(i)/1200
            _,heading,active:=aquariumRouteAt(route,phase)
            if !active {continue}
            if previousDirection!=0 && heading!=previousDirection {turnCount++}
            previousDirection=heading
            activeCount++
        }
        if activeCount==0 || turnCount==0 {
            t.Fatalf("species %d did not turn during patrol",kind)
        }
        if _,_,active:=aquariumRouteAt(route,route.travel+1);active {
            t.Fatalf("species %d never departed",kind)
        }
    }
}

func TestAquariumVisitingSpeciesAndSchoolsLeaveBetweenAppearances(t *testing.T) {
    species:=map[aquariumKind]bool{}
    schoolPresent,schoolAbsent:=false,false
    for second:=0;second<=1400;second+=13 {
        swimmers:=aquariumSwimmers(135,36,float64(second),441)
        visitors:=0
        for _,fish:=range swimmers {
            if fish.x>=135 || fish.x+aquariumFishWidth(fish)<=0 {continue}
            if fish.school {visitors++}
            if !fish.school && fish.kind!=aquariumMinnow &&
                fish.kind!=aquariumEel && fish.kind!=aquariumMuskie {
                species[fish.kind]=true
            }
        }
        if visitors>0 {schoolPresent=true} else {schoolAbsent=true}
    }
    if len(species)<4 {
        t.Fatalf("different species did not replace departed visitors: only %d species",len(species))
    }
    if !schoolPresent || !schoolAbsent {
        t.Fatalf("schools failed to arrive and leave: present=%v absent=%v",schoolPresent,schoolAbsent)
    }
}

func TestAquariumMinnowsAvoidHungryPredators(t *testing.T) {
    fish:=[]aquariumSwimmer{
        {id:1,kind:aquariumMinnow,x:40,y:12},
        {id:2,kind:aquariumPerch,x:41,y:13,mouthOpen:true},
    }
    aquariumMinnowAvoidance(fish,32)
    if fish[0].y>=12 {
        t.Fatalf("minnow didn't steer away from approaching perch: y=%d",fish[0].y)
    }
    safe:=[]aquariumSwimmer{
        {id:1,kind:aquariumMinnow,x:40,y:12},
        {id:2,kind:aquariumPerch,x:41,y:13,mouthOpen:false},
    }
    aquariumMinnowAvoidance(safe,32)
    if safe[0].y!=12 {t.Fatal("minnow reacted to a harmless closed-mouth perch")}
}

func TestAquariumEelHeadLeadsAndTailFollowsDepthChanges(t *testing.T) {
    seed:=uint64(12345)
    f:=aquariumSwimmer{
        kind:aquariumEel,length:18,direction:1,seed:seed,baseY:14,
    }
    speed:=3.5
    f.route=aquariumMakeRoute(160,aquariumFishWidth(f),1,speed,seed,2)
    foundCurve:=false
    for frame:=150;frame<1000;frame++ {
        phase:=float64(frame)/30
        if phase>=f.route.travel {break}
        x,direction,active:=aquariumRouteAt(f.route,phase)
        if !active || direction!=1 {continue}
        f.x=int(x)
        f.direction=direction
        f.routePhase=phase
        points:=aquariumEelBody(f,phase,40)
        if len(points)<12 {continue}
        rows:=map[int]bool{}
        for _,point:=range points {rows[point.y]=true}
        if len(rows)<2 {continue}
        head:=points[len(points)-1]
        if head.y!=aquariumEelHeadY(f,phase,40) {
            t.Fatalf("eel head detached from path: %d vs %d",head.y,aquariumEelHeadY(f,phase,40))
        }
        for _,point:=range points {
            if point.glyph!='~' {continue}
            if point.x< -aquariumFishWidth(f)-5 || point.x>180 {
                t.Fatalf("eel body jumped outside path: %d",point.x)
            }
        }
        foundCurve=true
        break
    }
    if !foundCurve {t.Fatal("eel tail did not curve smoothly along its head's previous path")}
}

func TestAquariumEelHeadMovesAllThreeGlyphsTogether(t *testing.T) {
    seed:=uint64(12345)
    for _,direction:=range []int{-1,1} {
        fish:=aquariumSwimmer{
            kind:aquariumEel,length:18,direction:direction,
            seed:seed,baseY:17,
        }
        speed:=aquariumFishSpeed(aquariumEel,seed+1)*3.2
        fish.route=aquariumMakeRoute(160,aquariumFishWidth(fish),direction,speed,seed,2)
        sawHeadShift:=false
        previousHeadY:=-1
        sawTailLag:=false
        for frame:=80;frame<1500;frame++ {
            seconds:=float64(frame)/30
            if seconds>=fish.route.travel {break}
            x,heading,active:=aquariumRouteAt(fish.route,seconds)
            if !active || heading!=direction {continue}
            fish.x=int(x)
            fish.direction=direction
            fish.routePhase=seconds
            points:=aquariumEelBody(fish,seconds,40)
            if len(points)<8 {continue}
            art:=[]rune(aquariumFishArt(fish)[0])
            start:=len(art)-3
            if direction<0 {start=0}
            var head [3]aquariumEelPoint
            for j:=0;j<3;j++ {
                found:=false
                for _,point:=range points {
                    if point.glyph==art[start+j] && point.x==fish.x+start+j {
                        head[j]=point
                        found=true
                        break
                    }
                }
                if !found {t.Fatalf("direction %d head rune %q missing at column %d",direction,art[start+j],fish.x+start+j)}
            }
            if head[0].y!=head[1].y || head[1].y!=head[2].y {
                t.Fatalf("direction %d eel head split across rows: %+v",direction,head)
            }
            if previousHeadY>=0 && previousHeadY!=head[0].y {sawHeadShift=true}
            previousHeadY=head[0].y
            for _,point:=range points {
                if point.glyph=='~' && point.y!=head[0].y {sawTailLag=true}
            }
            if sawHeadShift && sawTailLag {break}
        }
        if !sawHeadShift {t.Fatalf("direction %d head never changed depth",direction)}
        if !sawTailLag {t.Fatalf("direction %d tail never followed a different depth",direction)}
    }
}

func TestAquariumEelSpeedAndFishDiversity(t *testing.T) {
    seed:=uint64(211)
    baseline:=aquariumFishSpeed(aquariumEel,seed+1)*1.5
    increased:=aquariumFishSpeed(aquariumEel,seed+1)*3.2
    if increased<2*baseline {t.Fatalf("eel speed %.2f not twice the old %.2f",increased,baseline)}
    shades:=map[string]bool{}
    for id:=uint64(1);id<=96;id++ {
        f:=aquariumSwimmer{kind:aquariumMinnow,seed:chatFXHash(id),direction:1}
        shade:=aquariumFishGlyphColor(f,0,0,'>')
        shades[shade]=true
        if shade!=aquariumFishGlyphColor(f,0,1,'<') {
            t.Fatal("minnow changed colour between body segments")
        }
    }
    if len(shades)<5 {t.Fatalf("minnow palette lacks individual variation: %d shades",len(shades))}
    schoolShades:=map[string]bool{}
    for id:=uint64(1);id<=12;id++ {
        sharedSeed:=uint64(934)
        fish:=aquariumSwimmer{
            kind:aquariumMinnow,seed:sharedSeed,
            colorSeed:chatFXHash(sharedSeed+id*0x9e3779b97f4a7c15),
        }
        schoolShades[aquariumFishGlyphColor(fish,0,0,'>')]=true
    }
    if len(schoolShades)<3 {
        t.Fatalf("school members weren't individually shaded: %d colours",len(schoolShades))
    }
    for _,height:=range []int{24,40,70} {
        for seed:=uint64(0);seed<128;seed++ {
            f:=aquariumSwimmer{kind:aquariumWalleye,seed:seed,baseY:aquariumFishDepth(aquariumWalleye,seed,height)}
            if got:=aquariumFishY(f,0,height);got<height*2/3 {
                t.Fatalf("walleye wandered above lower tank: y %d, height %d",got,height)
            }
        }
    }
}

func TestAquariumSchoolsOccasionallyCrossWithoutRemainingPermanent(t *testing.T) {
    m:=newMessageInteractionTestModel(130)
    m.fxRoom=chatFXState{name:"aquarium"}
    m.fxSalt=177
    // School fish are coloured the same as individuals, but the number
    // of visible fish (particularly on long paths) should vary with time.
    populations:=map[int]bool{}
    for second:=0;second<160;second+=4 {
        m.fxSimSteps=uint64(second*30)
        rows:=m.aquariumPositions(128,32)
        heads:=0
        for _,row:=range rows {
            for _,cell:=range row {
                if cell.glyph=='<' || cell.glyph=='>' {heads++}
            }
        }
        populations[heads]=true
    }
    if len(populations)<3 {
        t.Fatalf("schools never entered or exited: observed %d fish populations",len(populations))
    }
}

func TestAquariumKelpAnchorsAndLengthsVary(t *testing.T) {
    m:=newMessageInteractionTestModel(180)
    m.fxRoom=chatFXState{name:"aquarium"}
    m.fxSalt=117
    initial:=m.aquariumPositions(178,38)
    // Bottom stalk positions are derived only from the seed.
    // Ignore other glyphs, which can swim in and out of those rows.
    bottom:=map[int]bool{}
    for x,cell:=range initial[37] {
        if cell.glyph=='/' || cell.glyph=='\\' {bottom[x]=true}
    }
    if len(bottom)<3 {t.Fatalf("not enough bottom-rooted kelp stalks: %d",len(bottom))}
    m.fxSimSteps=300
    later:=m.aquariumPositions(178,38)
    retained:=0
    for x:=range bottom {
        if cell,ok:=later[37][x];ok && (cell.glyph=='/' || cell.glyph=='\\') {retained++}
    }
    if retained<len(bottom)-1 {
        t.Fatalf("kelp roots drifted with the current: %d/%d",retained,len(bottom))
    }
    lengthRows:=map[int]bool{}
    for y:=20;y<38;y++ {
        for _,cell:=range initial[y] {
            if cell.glyph=='/' || cell.glyph=='\\' {lengthRows[y]=true}
        }
    }
    if len(lengthRows)<4 {t.Fatal("kelp lacked varied stalk heights")}
}

func TestAquariumNeverOverwritesChatMessages(t *testing.T) {
    m:=newMessageInteractionTestModel(100)
    m.fxRoom=chatFXState{name:"aquarium"}
    m.fxSalt=42
    original:=[]string{
        "  [12:30] Alice: <>< fish should never touch this text!",
        "  [12:31] Bob: Seaweed / \\ is pretty neat.",
        strings.Repeat(" ",98),
        strings.Repeat(" ",98),
        strings.Repeat(" ",98),
        strings.Repeat(" ",98),
        strings.Repeat(" ",98),
        strings.Repeat(" ",98),
    }
    for _,tick:=range []uint64{0,30,120,900} {
        m.fxSimSteps=tick
        rows:=append([]string(nil),original...)
        m.renderChatFXBackground(rows,98)
        for y:=0;y<2;y++ {
            if !strings.HasPrefix(stripANSISequences(rows[y]),original[y]) {
                t.Fatalf("aquarium obscured chat at tick %d row %d: %q",tick,y,stripANSISequences(rows[y]))
            }
        }
    }
    m.fxDisabled=true
    rows:=append([]string(nil),original...)
    m.renderChatFXBackground(rows,98)
    if !reflect.DeepEqual(rows,original) {t.Fatal("disablefx did not hide aquarium")}
}

func TestAquariumCommandsLocalAndRoom(t *testing.T) {
    m:=newMessageInteractionTestModel(100)
    m.room="lobby"
    m.connected=true
    if !isChatFXCommand("/aquarium") {t.Fatal("aquarium command not registered")}
    var wire bytes.Buffer
    m.client=&wireClient{enc:json.NewEncoder(&wire)}
    m.chatFXCommand("/aquarium",[]string{"/aquarium","self"})
    if m.currentChatFX().name!="aquarium" || wire.Len()!=0 {
        t.Fatal("local aquarium was not local")
    }
    if m.fxAquariumSalt==0 {
        t.Fatal("aquarium did not generate a new layout seed on launch")
    }
    m.chatFXCommand("/stopfx",[]string{"/stopfx","self"})
    m.chatFXCommand("/aquarium",[]string{"/aquarium","35"})
    var packet Packet
    if err:=json.NewDecoder(&wire).Decode(&packet);err!=nil {t.Fatal(err)}
    if packet.Type!="fx_start" || packet.FX!="aquarium" || packet.FXSeconds!=35 {
        t.Fatalf("room aquarium command wrong: %+v",packet)
    }
    m.receiveChatFX(Packet{Type:"fx_state",FX:"aquarium"})
    if m.currentChatFX().name!="aquarium" {
        t.Fatal("aquarium room state was not recognized")
    }
}
