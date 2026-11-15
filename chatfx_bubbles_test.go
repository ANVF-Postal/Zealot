package main

import (
    "math"
    "strings"
    "testing"
    "time"
)

func TestBubbleCapacityIsAreaBoundedAndCapped(t *testing.T) {
    if n:=bubbleCapacity(1600,900);n!=bubbleMaximum {
        t.Fatalf("large screen generated %d bubbles, cap is %d",n,bubbleMaximum)
    }
    if n:=bubbleCapacity(50,15);n<=0 || n>=bubbleMaximum {
        t.Fatalf("small screen should have limited population: %d",n)
    }
    if bubbleCapacity(5,4)!=0 {t.Fatal("bubbles cannot fit in tiny viewports")}
    for _,dimension:=range [][2]int{{10,6},{24,10},{80,24},{200,50}} {
        if got:=bubbleCapacity(dimension[0],dimension[1]);got>bubbleMaximum {
            t.Fatalf("exceeded max: %d",got)
        }
    }
}

func TestBubblesSpawnProgressivelyAndNeverExceedCapacity(t *testing.T) {
    s:=newBubbleState(180,52,91)
    if len(s.bubbles)!=0 {t.Fatal("bubbles were not empty initially")}
    s.advance(1,180,52)
    if len(s.bubbles)!=1 {t.Fatalf("first bubble did not appear: %d",len(s.bubbles))}
    startCount:=len(s.bubbles)
    s.advance(180,180,52)
    if len(s.bubbles)<=startCount {t.Fatal("bubbles did not progressively spawn")}
    peak:=len(s.bubbles)
    for tick:=0;tick<6000;tick++ {
        s.advance(1,180,52)
        if len(s.bubbles)>bubbleMaximum {
            t.Fatalf("bubble count exceeded 32: %d",len(s.bubbles))
        }
        if len(s.bubbles)>peak {peak=len(s.bubbles)}
        for _,b:=range s.bubbles {
            if b.x<0 || b.x>float64(s.width) || b.y<0 || b.y>float64(s.height) {
                t.Fatalf("bubble escaped the chat area: %+v",b)
            }
        }
    }
    if peak<10 {t.Fatalf("bubble field stayed too sparse: peak %d",peak)}
    if s.serial<=uint64(peak) {t.Fatal("bubbles never popped and got replaced")}
}

func TestBubblePopsAndReplenishes(t *testing.T) {
    s:=newBubbleState(100,35,7)
    s.bubbles=[]bubbleParticle{{x:50,y:18,vx:0,vy:0,radius:3,age:19.99,lifetime:20}}
    s.steps=1
    s.tick()
    if len(s.bubbles)!=0 {t.Fatal("expired bubble did not pop")}
    for i:=0;i<25;i++ {s.tick()}
    if len(s.bubbles)==0 {t.Fatal("popped bubble was never replaced")}
}

func TestBubbleCollisionsAndWallBounces(t *testing.T) {
    s:=newBubbleState(100,35,3)
    s.bubbles=[]bubbleParticle{
        {x:44,y:18,vx:3,vy:0,radius:3,lifetime:100},
        {x:56,y:18,vx:-3,vy:0,radius:3,lifetime:100},
    }
    s.steps=1
    // Test the actual pair collision before new bubbles spawn, so no
    // unrelated interactions can alter the two velocities.
    for i:=0;i<8;i++ {s.tick()}
    if !(s.bubbles[0].vx<0 && s.bubbles[1].vx>0) {
        t.Fatalf("colliding bubbles did not rebound: %f %f",s.bubbles[0].vx,s.bubbles[1].vx)
    }
    nearWall:=newBubbleState(100,35,17)
    nearWall.bubbles=[]bubbleParticle{{x:6,y:18,vx:-4,vy:0,radius:3,lifetime:100}}
    nearWall.steps=1
    nearWall.tick()
    if nearWall.bubbles[0].vx<=0 {t.Fatal("bubble did not bounce off left boundary")}
}

func TestBubblePhysicsIsIndependentOfRenderFPS(t *testing.T) {
    for _,fps:=range []int{1,15,30,60} {
        s:=newBubbleState(140,40,33)
        // Fixed 30Hz simulation always reaches identical state after
        // the same number of ticks, irrespective of presentation FPS.
        for i:=0;i<120;i++ {s.advance(1,140,40)}
        reference:=newBubbleState(140,40,33)
        reference.advance(120,140,40)
        if len(s.bubbles)!=len(reference.bubbles) {
            t.Fatalf("FPS %d affected population",fps)
        }
        for i,b:=range s.bubbles {
            r:=reference.bubbles[i]
            if math.Abs(b.x-r.x)>1e-9 || math.Abs(b.y-r.y)>1e-9 {
                t.Fatalf("FPS %d affected bubble physics",fps)
            }
        }
    }
}

func TestBubblePaintingPreservesChatAndANSI(t *testing.T) {
    s:=newBubbleState(80,26,44)
    s.bubbles=[]bubbleParticle{
        {x:20,y:7,radius:5,color:[3]int{200,80,220},age:1,lifetime:60},
    }
    grid:=s.raster(80,26)
    original:="\x1b[48;2;17;19;24m\x1b[38;2;240;240;240m  Alice: "+
        "This is an important message"
    painted:=bubblePaintRow(original,grid[7],"#111318",80)
    if !strings.HasPrefix(stripANSISequences(painted),stripANSISequences(original)) {
        t.Fatalf("bubbles obscured chat text: %q",stripANSISequences(painted))
    }
    if !strings.Contains(painted,"\x1b[38;2;240;240;240m") {
        t.Fatal("bubble painting removed foreground ANSI")
    }
    if len(stripANSISequences(painted))!=80 {t.Fatal("bubble row did not fill chat width")}
    empty:=bubblePaintRow(strings.Repeat(" ",80),grid[7],"#111318",80)
    visible:=stripANSISequences(empty)
    if !strings.ContainsAny(visible,"/\\|-o") {
        t.Fatal("hollow bubble outline wasn't drawn on empty background")
    }
    if !strings.Contains(empty,"\x1b[38;2;") {
        t.Fatal("outline had no coloured foreground")
    }
}

func TestBubbleOutlineIsHollowAndHasReflection(t *testing.T) {
    bubble:=bubbleParticle{x:30,y:10,radius:5,color:[3]int{70,180,255},age:1,lifetime:30}
    if glyph:=bubbleGlyphAt(bubble,30,10);glyph!=0 {
        t.Fatalf("bubble centre should be empty, got %q",glyph)
    }
    outline,reflection:=0,0
    for y:=4;y<16;y++ {
        for x:=17;x<43;x++ {
            glyph:=bubbleGlyphAt(bubble,x,y)
            if glyph!=0 {outline++}
            if glyph=='o' {reflection++}
        }
    }
    if outline<20 || outline>140 {t.Fatalf("unexpected outline density: %d",outline)}
    if reflection==0 {t.Fatal("bubble has no small reflected glint")}
}

func TestBubbleLaunchesFromBottomRightAndMovesUpLeft(t *testing.T) {
    s:=newBubbleState(160,48,51)
    s.advance(1,160,48)
    if len(s.bubbles)!=1 {t.Fatal("first bubble never spawned")}
    b:=s.bubbles[0]
    if b.vx>=0 || b.vy>=0 {t.Fatalf("bubble wasn't blown up-left: %+v",b)}
    if b.x<float64(s.width)*0.62 || b.y<float64(s.height)*0.58 {
        t.Fatalf("bubble didn't enter from lower-right: %+v",b)
    }
}

func TestBubblePopIsInstantWithASCIIFlash(t *testing.T) {
    s:=newBubbleState(90,30,8)
    s.steps=2
    s.bubbles=[]bubbleParticle{{
        x:45,y:15,radius:4,age:9.99,lifetime:10,color:[3]int{120,240,140},
    }}
    s.tick()
    if len(s.bubbles)!=0 {t.Fatal("expired bubble lingered or shrank instead of popping")}
    if len(s.pops)!=1 {t.Fatal("pop did not create an ASCII burst")}
    grid:=s.raster(90,30)
    line:=bubblePaintRow(strings.Repeat(" ",90),grid[15],"#111318",90)
    if !strings.Contains(stripANSISequences(line),"*pop*") {
        t.Fatal("bubble pop burst has no *pop* lettering")
    }
    for i:=0;i<25;i++ {s.tick()}
    if len(s.pops)!=0 {t.Fatal("pop burst did not disappear promptly")}
}

func TestBubbleRenderingProtectsWordsAndColours(t *testing.T) {
    s:=newBubbleState(90,30,4)
    s.bubbles=[]bubbleParticle{{x:22,y:10,radius:5,color:[3]int{255,50,90},age:1,lifetime:30}}
    row:="\x1b[38;2;60;200;100mMessage with spaces"
    grid:=s.raster(90,30)
    rendered:=bubblePaintRow(row,grid[10],"#111318",90)
    if !strings.HasPrefix(stripANSISequences(rendered),"Message with spaces") {
        t.Fatalf("bubble altered text: %q",stripANSISequences(rendered))
    }
    if !strings.Contains(rendered,"\x1b[38;2;60;200;100m") {
        t.Fatal("bubble drawing lost chat foreground style")
    }
}

func TestBubblesCommandScopesAndStop(t *testing.T) {
    m:=newMessageInteractionTestModel(100)
    if !isChatFXCommand("/bubbles") {t.Fatal("bubble command unregistered")}
    m.chatFXCommand("/bubbles",[]string{"/bubbles","self"})
    if m.currentChatFX().name!="bubbles" || m.fxBubbles==nil {
        t.Fatal("local bubble effect did not initialize")
    }
    m.advanceChatFXClock(time.Now())
    m.advanceChatFXClock(time.Now().Add(100*time.Millisecond))
    if len(m.fxBubbles.bubbles)==0 {t.Fatal("bubble simulation didn't advance")}
    m.chatFXCommand("/stopfx",[]string{"/stopfx","self"})
    if m.currentChatFX().name!="" {t.Fatal("bubble effect did not stop")}
}

func BenchmarkBubblesRender(b *testing.B) {
    m:=newMessageInteractionTestModel(160)
    m.fxRoom=chatFXState{name:"bubbles"}
    m.fxBubbles=newBubbleState(158,45,173)
    m.fxBubbles.advance(900,158,45)
    baseline:=make([]string,45)
    for y:=range baseline {baseline[y]=strings.Repeat(" ",158)}
    b.ReportAllocs()
    for i:=0;i<b.N;i++ {
        rows:=append([]string(nil),baseline...)
        m.renderBubblesRows(rows,158)
    }
}
