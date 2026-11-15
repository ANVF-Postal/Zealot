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

func TestAquariumFishSpeciesAreDistinctAndDirectional(t *testing.T) {
    if len(aquariumSpecies)<4 {t.Fatal("expected several fish species")}
    shapes:=map[string]bool{}
    speeds:=map[float64]bool{}
    for _,species:=range aquariumSpecies {
        if len(species.left)<3 || len(species.right)<3 ||
            species.left[0]!='<' || species.right[len(species.right)-1]!='>' {
            t.Fatalf("invalid directional fish: %+v",species)
        }
        if len(species.left)!=len(species.right) {t.Fatal("species changes width when turning")}
        if shapes[species.left] || shapes[species.right] {
            t.Fatalf("duplicated species shape: %+v",species)
        }
        shapes[species.left]=true
        shapes[species.right]=true
        speeds[species.speed]=true
        if aquariumFishShape(species,-1)!=species.left || aquariumFishShape(species,1)!=species.right {
            t.Fatal("fish direction selection failed")
        }
    }
    if len(speeds)<4 {t.Fatalf("species speeds insufficiently varied: %d",len(speeds))}
}

func TestAquariumSchoolsHaveDistinctTravelAndRestWindows(t *testing.T) {
    phase,active:=aquariumSchoolPhase(4,0,12,18)
    if !active || phase!=4 {t.Fatalf("school absent during crossing: %v, %.1f",active,phase)}
    if _,active:=aquariumSchoolPhase(16,0,12,18);active {
        t.Fatal("school did not leave for its resting interval")
    }
    if phase,active:=aquariumSchoolPhase(34,0,12,18);!active || phase!=4 {
        t.Fatalf("school did not re-enter after rest: %v, %.1f",active,phase)
    }
    if _,active:=aquariumSchoolPhase(0,17,12,18);active {
        t.Fatal("offset school appeared during its inactive interval")
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
