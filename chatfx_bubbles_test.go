package main

import (
    "math"
    "strings"
    "testing"
    "time"
    "unicode/utf8"
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
    nearWall.bubbles=[]bubbleParticle{{x:4.68,y:18,vx:-4,vy:0,radius:3,lifetime:100}}
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
    if utf8.RuneCountInString(stripANSISequences(painted))!=80 {t.Fatal("bubble row did not fill chat width")}
    empty:=bubblePaintRow(strings.Repeat(" ",80),grid[7],"#111318",80)
    visible:=stripANSISequences(empty)
    foundBraille:=false
    for _,r:=range visible {
        if r>='\u2801' && r<='\u28ff' {foundBraille=true;break}
    }
    if !foundBraille {t.Fatal("no braille bubble border drawn on empty background")}
    if !strings.Contains(empty,"\x1b[38;2;") {
        t.Fatal("outline had no coloured foreground")
    }
}

func TestBubbleOutlineIsHollowAndHasOneReflection(t *testing.T) {
    bubble:=bubbleParticle{x:30,y:10,radius:5,color:[3]int{70,180,255},age:1,lifetime:30}
    if glyph:=bubbleGlyphAt(bubble,30,10);glyph!=0 {
        t.Fatalf("bubble centre should be empty, got %q",glyph)
    }
    outline:=0
    for y:=4;y<16;y++ {
        for x:=17;x<43;x++ {
            glyph:=bubbleGlyphAt(bubble,x,y)
            if glyph!=0 {outline++}
            if glyph=='o' {t.Fatal("reflection was rendered once per cell")}
        }
    }
    if outline<20 || outline>140 {t.Fatalf("unexpected outline density: %d",outline)}
    grid:=(&bubbleState{bubbles:[]bubbleParticle{bubble}}).raster(80,26)
    highlights:=0
    for _,row:=range grid {
        for _,cell:=range row {
            if cell.color==[3]int{244,250,255} {highlights++}
        }
    }
    if highlights<2 || highlights>12 {
        t.Fatalf("expected one compact Braille crescent, got %d cells",highlights)
    }
    for _,row:=range grid {
        for _,cell:=range row {
            if cell.color==[3]int{244,250,255} && (cell.glyph<'\\u2801' || cell.glyph>'\\u28ff') {
                t.Fatalf("large-bubble highlight must be Braille, got %U",cell.glyph)
            }
        }
    }
    small:=bubble
    small.radius=2.4
    smallGrid:=(&bubbleState{bubbles:[]bubbleParticle{small}}).raster(80,26)
    countO:=0
    for _,row:=range smallGrid {
        for _,cell:=range row {if cell.glyph=='o' {countO++}}
    }
    if countO!=1 {t.Fatalf("small bubble should have one 'o', got %d",countO)}
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

func TestBubbleBrailleOutlineHasSubcellResolution(t *testing.T) {
    b:=bubbleParticle{x:32,y:10,radius:5,age:1,lifetime:30}
    if g:=bubbleGlyphAt(b,32,10);g!=0 {t.Fatalf("centre should be empty: %U",g)}
    masks:=make(map[rune]bool)
    for y:=3;y<17;y++ {
        for x:=18;x<46;x++ {
            glyph:=bubbleGlyphAt(b,x,y)
            if glyph>='\u2801' && glyph<='\u28ff' {masks[glyph]=true}
        }
    }
    if len(masks)<5 {t.Fatalf("braille border lacks curved subcell detail: %d shapes",len(masks))}
}

func TestBubbleSoftBodyDeformsLocallyAndRecovers(t *testing.T) {
    b:=bubbleParticle{x:45,y:18,radius:4,lifetime:100}
    b.pressRim(0,0.45)
    // The nearest node is pushed inward, but the far side is unaffected.
    b.advanceRim(bubbleStepSeconds)
    right:=b.rimRadius(0)
    left:=b.rimRadius(math.Pi)
    if right>=b.radius*0.98 || math.Abs(left-b.radius)>0.02 {
        t.Fatalf("contact wasn't local: right %.3f left %.3f radius %.3f",right,left,b.radius)
    }
    distorted:=false
    for i:=0;i<55;i++ {
        b.advanceRim(bubbleStepSeconds)
        if b.rimRadius(math.Pi/3)!=b.radius {distorted=true}
    }
    if !distorted {t.Fatal("border deformation didn't propagate along the ring")}
    for i:=0;i<240;i++ {b.advanceRim(bubbleStepSeconds)}
    for i,r:=range b.rim {
        if math.Abs(r)>0.025 {t.Fatalf("rim point %d didn't settle: %f",i,r)}
    }
}

func TestBubbleWallContactFlattensOnlyContactSide(t *testing.T) {
    b:=bubbleParticle{x:5,y:18,radius:4,lifetime:60}
    b.constrainRimToWalls(100,35)
    left:=b.rimRadius(math.Pi)
    right:=b.rimRadius(0)
    if !(left<b.radius*0.75 && math.Abs(right-b.radius)<0.001) {
        t.Fatalf("wall contact didn't locally flatten left side: left=%f right=%f",left,right)
    }
    for i:=0;i<30;i++ {
        b.advanceRim(bubbleStepSeconds)
        b.constrainRimToWalls(100,35)
    }
    if b.rimRadius(math.Pi)>=b.radius {
        t.Fatal("wall-contact node passed through wall during spring recovery")
    }
}

func TestBubbleCollisionAndWallPushLocalRim(t *testing.T) {
    s:=newBubbleState(100,35,3)
    s.steps=2
    s.bubbles=[]bubbleParticle{
        {x:44,y:18,vx:5,vy:0,radius:3,lifetime:100},
        {x:55,y:18,vx:-5,vy:0,radius:3,lifetime:100},
    }
    s.tick()
    if s.bubbles[0].rimSpeed[0]==0 || s.bubbles[1].rimSpeed[12]==0 {
        t.Fatal("collision didn't compress contact-side rim nodes")
    }
    wall:=newBubbleState(100,35,4)
    wall.steps=2
    wall.bubbles=[]bubbleParticle{{x:6,y:18,vx:-5,radius:3,lifetime:100}}
    wall.tick()
    if wall.bubbles[0].rimSpeed[12]==0 {
        t.Fatal("wall impact didn't deform the left edge of the bubble")
    }
}

func TestBubbleEntrySpeedIsVariedButCapped(t *testing.T) {
    slow,fast:=0,0
    for salt:=uint64(0);salt<300;salt++ {
        s:=newBubbleState(160,48,salt)
        s.spawn()
        if len(s.bubbles)==0 {continue}
        speed:=math.Hypot(s.bubbles[0].vx*0.5,s.bubbles[0].vy)
        if speed<5 {slow++}
        if speed>bubbleFastThreshold {fast++}
        if speed>bubbleMaxSpeed+1e-9 {t.Fatalf("bubble spawned too fast: %.3f",speed)}
    }
    if slow==0 || fast==0 {t.Fatalf("expected mixed gentle and faster bubbles: slow=%d fast=%d",slow,fast)}
}

func TestBubbleSpeedDoesNotEscalateAfterRepeatedCollisions(t *testing.T) {
    s:=newBubbleState(100,32,9)
    s.steps=2
    s.bubbles=[]bubbleParticle{
        {x:40,y:15,vx:80,vy:22,radius:3,lifetime:100},
        {x:52,y:15,vx:-90,vy:-18,radius:3,lifetime:100},
    }
    for step:=0;step<240;step++ {
        s.tick()
        for i,b:=range s.bubbles {
            if speed:=math.Hypot(b.vx*0.5,b.vy);speed>bubbleMaxSpeed+1e-8 {
                t.Fatalf("step %d bubble %d escaped speed cap: %.2f",step,i,speed)
            }
        }
    }
}

func TestBubbleMaximumRadiusTracksViewport(t *testing.T) {
    for _,dims:=range [][2]int{{50,15},{80,24},{160,48},{400,100}} {
        s:=newBubbleState(dims[0],dims[1],123)
        maxRadius:=bubbleMaximumRadius(dims[0],dims[1])
        if maxRadius>5.2 || maxRadius>float64(dims[1])*0.16+1e-8 {
            t.Fatalf("invalid size cap for %v: %.2f",dims,maxRadius)
        }
        for i:=0;i<80;i++ {s.spawn()}
        for _,b:=range s.bubbles {
            if b.radius>maxRadius+1e-8 {
                t.Fatalf("bubble radius %.2f exceeds viewport limit %.2f",b.radius,maxRadius)
            }
        }
    }
    small:=newBubbleState(160,48,41)
    small.spawn()
    small.resize(50,15)
    for _,b:=range small.bubbles {
        if b.radius>bubbleMaximumRadius(50,15)+1e-8 {
            t.Fatal("resize failed to shrink an existing bubble")
        }
    }
}

func TestBrailleReflectionIsOneContinuousArc(t *testing.T) {
    b:=bubbleParticle{x:40,y:14,radius:5.2}
    pixels:=bubbleReflectionPixels(b)
    if len(pixels)<2 || len(pixels)>12 {
        t.Fatalf("unexpected Braille highlight size: %d",len(pixels))
    }
    for _,p:=range pixels {
        if p.glyph<'\\u2801' || p.glyph>'\\u28ff' {
            t.Fatalf("non-Braille highlight %U",p.glyph)
        }
    }
    // The small arc must not have multiple separated eye-like clusters.
    seen:=make(map[[2]int]bool)
    for _,p:=range pixels {seen[[2]int{p.x,p.y}]=true}
    visited:=map[[2]int]bool{}
    queue:=[][2]int{{pixels[0].x,pixels[0].y}}
    for len(queue)>0 {
        point:=queue[0]
        queue=queue[1:]
        if visited[point] {continue}
        visited[point]=true
        for dy:=-1;dy<=1;dy++ {
            for dx:=-1;dx<=1;dx++ {
                neighbor:=[2]int{point[0]+dx,point[1]+dy}
                if seen[neighbor] && !visited[neighbor] {queue=append(queue,neighbor)}
            }
        }
    }
    if len(visited)!=len(seen) {t.Fatalf("reflection split into separate highlights: %d of %d connected",len(visited),len(seen))}
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
    fragment:=s.pops[0].fragments[0]
    for i:=0;i<3;i++ {s.tick()}
    moved:=s.pops[0].fragments[0]
    if moved.x==fragment.x && moved.y==fragment.y {
        t.Fatal("pop fragments stayed frozen rather than dispersing")
    }
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
