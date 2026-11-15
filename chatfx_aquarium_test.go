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
        if reversed:=aquariumFishArt(f)[0];reversed!=aquariumMirror(art[0]) {
            t.Fatalf("species %d wasn't mirrored correctly",tc.kind)
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
        big:=aquariumSwimmer{id:3,kind:hunter,direction:1,x:20,y:8,length:6}
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

func TestAquariumPikeStripesOscillateAndLengthsVary(t *testing.T) {
    short:=aquariumSwimmer{kind:aquariumPike,direction:1,length:3}
    long:=aquariumSwimmer{kind:aquariumPike,direction:1,length:9}
    if aquariumFishWidth(long)<=aquariumFishWidth(short) {
        t.Fatal("pike body lengths don't vary")
    }
    short.stripePhase=0
    first:=aquariumFishGlyphColor(short,0,2,'(')
    short.stripePhase=1
    if aquariumFishGlyphColor(short,0,2,'(')==first {
        t.Fatal("pike stripe colours never oscillate")
    }
    a:=aquariumSwimmer{kind:aquariumWalleye,direction:1,length:3}
    b:=aquariumSwimmer{kind:aquariumWalleye,direction:1,length:10}
    if aquariumFishWidth(b)<=aquariumFishWidth(a) {t.Fatal("walleye body lengths don't vary")}
}

func TestAquariumEelBendsTailBetweenRows(t *testing.T) {
    m:=newMessageInteractionTestModel(100)
    m.fxSalt=4242
    found:=false
    for sec:=0;sec<720;sec++ {
        fish:=aquariumSwimmers(98,30,float64(sec),4242)
        eelVisible:=false
        for _,f:=range fish {
            if f.kind==aquariumEel && f.x>=0 && f.x+aquariumFishWidth(f)<98 {
                if f.tailBend!=0 {
                    eelVisible=true
                }
            }
        }
        if !eelVisible {continue}
        output:=make(map[int]map[int]rune)
        m.renderAquariumFish(98,30,float64(sec),4242,func(x,y int,glyph rune,color string) {
            if output[y]==nil {output[y]=map[int]rune{}}
            output[y][x]=glyph
        })
        for _,f:=range fish {
            if f.kind!=aquariumEel || f.x<0 || f.x+aquariumFishWidth(f)>=98 {continue}
            tailRows:=map[int]bool{}
            for y,row:=range output {
                for x,glyph:=range row {
                    if x>=f.x+aquariumFishWidth(f)*2/3 && x<f.x+aquariumFishWidth(f) &&
                        (glyph=='~' || glyph=='*' || glyph=='\\' || glyph=='/') {
                        tailRows[y]=true
                    }
                }
            }
            if len(tailRows)>=2 {found=true;break}
        }
        if found {break}
    }
    if !found {t.Fatal("eel never left a bent tail on a second row")}
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
