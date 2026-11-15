package main

import "math"

const (
    bubbleMaximum = 32
    bubbleStepSeconds = 1.0 / 30.0
)

type bubbleParticle struct {
    x,y float64 // terminal cells (x is approximately twice as dense as y)
    vx,vy float64 // terminal cells per second
    radius float64 // vertical cell radius
    age,lifetime float64
    color [3]int
    wobble float64 // signed, spring-driven squash/stretch
    wobbleVelocity float64
}

type bubblePop struct {
    x,y float64
    age float64
    color [3]int
}

type bubbleState struct {
    width,height int
    salt uint64
    steps uint64
    serial uint64
    bubbles []bubbleParticle
    pops []bubblePop
}

func bubbleCapacity(width,height int) int {
    if width<8 || height<5 {return 0}
    // A large bubble occupies approximately 2*pi*r^2 cells. Don't let
    // population grow faster than viewport area or exceed 32.
    area:=width*height
    target:=area/165
    if target<1 {target=1}
    return min(bubbleMaximum,target)
}

func newBubbleState(width,height int,salt uint64) *bubbleState {
    return &bubbleState{width:width,height:height,salt:salt}
}

func (s *bubbleState) resize(width,height int) {
    if s.width==width && s.height==height {return}
    oldW,oldH:=max(1,s.width),max(1,s.height)
    maxRadius:=math.Min(6.5,math.Min(float64(width)/5,float64(height)/2.8))
    kept:=s.bubbles[:0]
    for _,b:=range s.bubbles {
        if maxRadius<2.2 {continue}
        b.radius=math.Min(b.radius,maxRadius)
        b.x*=float64(width)/float64(oldW)
        b.y*=float64(height)/float64(oldH)
        b.x=math.Max(b.radius*2,math.Min(float64(width)-b.radius*2,b.x))
        b.y=math.Max(b.radius,math.Min(float64(height)-b.radius,b.y))
        kept=append(kept,b)
    }
    s.bubbles=kept
    s.pops=nil // discard old viewport-space bursts on resize
    s.width,s.height=width,height
    capacity:=bubbleCapacity(width,height)
    if len(s.bubbles)>capacity {s.bubbles=s.bubbles[:capacity]}
}

func (s *bubbleState) spawn() {
    if len(s.bubbles)>=bubbleCapacity(s.width,s.height) {return}
    seed:=chatFXHash(s.salt+0x916f6a1d* (s.serial+1))
    s.serial++
    maxRadius:=math.Min(6.5,math.Min(float64(s.width)/5,float64(s.height)/2.8))
    if maxRadius<2.2 {return}
    radius:=2.2+(maxRadius-2.2)*chatFXUnit(seed+1)
    // Bubbles enter from the lower-right, then carry up-left momentum.
    // Broaden the launch area as it fills, avoiding a pile-up in the corner.
    for attempt:=uint64(0);attempt<36;attempt++ {
        value:=chatFXHash(seed+attempt*0x9e3779b97f4a7c15)
        spread:=math.Min(0.85,0.18+float64(len(s.bubbles))*0.024)
        x:=float64(s.width)-radius*2-
            (float64(s.width)-radius*4)*spread*chatFXUnit(value+2)
        y:=float64(s.height)-radius-
            (float64(s.height)-radius*2)*spread*chatFXUnit(value+3)
        clear:=true
        for _,other:=range s.bubbles {
            dx:=(x-other.x)*0.5
            dy:=y-other.y
            if dx*dx+dy*dy<math.Pow(radius+other.radius+0.2,2) {
                clear=false
                break
            }
        }
        if !clear {continue}
        // Most bubbles glide, some race dramatically into the room.
        // Speeds use terminal cells per second, in the same fixed-step
        // simulation so /fxfps cannot alter their trajectories.
        speed:=1.3+2.5*chatFXUnit(value+5)
        if chatFXUnit(value+10)>0.81 {speed*=3.1+1.8*chatFXUnit(value+11)}
        vx:=-(1.6+1.2*chatFXUnit(value+4))*speed
        vy:=-(0.55+0.8*chatFXUnit(value+9))*speed
        red,green,blue:=hsvToRGB(chatFXUnit(value+6),0.55+0.35*chatFXUnit(value+7),0.90)
        s.bubbles=append(s.bubbles,bubbleParticle{
            x:x,y:y,vx:vx,vy:vy,
            radius:radius,age:0,
            lifetime:11+23*chatFXUnit(value+8),
            color:[3]int{red,green,blue},
        })
        return
    }
}

func (s *bubbleState) tick() {
    if s.width<8 || s.height<5 {return}
    s.steps++
    dt:=bubbleStepSeconds
    // Short ASCII bursts survive the instantaneous disappearance of a bubble.
    pops:=s.pops[:0]
    for _,p:=range s.pops {
        p.age+=dt
        if p.age<0.65 {pops=append(pops,p)}
    }
    s.pops=pops
    alive:=s.bubbles[:0]
    for _,bubble:=range s.bubbles {
        bubble.age+=dt
        if bubble.age>=bubble.lifetime {
            if len(s.pops)<bubbleMaximum {
                s.pops=append(s.pops,bubblePop{x:bubble.x,y:bubble.y,color:bubble.color})
            }
            continue // immediate pop: the sphere is gone this frame
        }
        // Underdamped spring: squash, overshoot, then settle.
        bubble.wobbleVelocity+=(-58*bubble.wobble-9*bubble.wobbleVelocity)*dt
        bubble.wobble+=bubble.wobbleVelocity*dt
        bubble.wobble=math.Max(-0.29,math.Min(0.29,bubble.wobble))
        bubble.x+=bubble.vx*dt
        bubble.y+=bubble.vy*dt
        minX,maxX:=bubble.radius*2,float64(s.width)-bubble.radius*2
        minY,maxY:=bubble.radius,float64(s.height)-bubble.radius
        if bubble.x<minX {bubble.x=minX;bubble.vx=math.Abs(bubble.vx);bubble.wobbleVelocity-=4.0}
        if bubble.x>maxX {bubble.x=maxX;bubble.vx=-math.Abs(bubble.vx);bubble.wobbleVelocity+=4.0}
        if bubble.y<minY {bubble.y=minY;bubble.vy=math.Abs(bubble.vy);bubble.wobbleVelocity+=3.0}
        if bubble.y>maxY {bubble.y=maxY;bubble.vy=-math.Abs(bubble.vy);bubble.wobbleVelocity-=3.0}
        alive=append(alive,bubble)
    }
    s.bubbles=alive

    // Physical sphere collisions, with separation to avoid sticking.
    // The x axis is compressed by two to match monospace terminal cells.
    for i:=0;i<len(s.bubbles);i++ {
        for j:=i+1;j<len(s.bubbles);j++ {
            a,b:=&s.bubbles[i],&s.bubbles[j]
            dx:=(b.x-a.x)*0.5
            dy:=b.y-a.y
            dist:=math.Hypot(dx,dy)
            required:=a.radius+b.radius
            if dist>=required {continue}
            if dist<0.0001 {dx,dy,dist=1,0,1}
            nx,ny:=dx/dist,dy/dist
            overlap:=(required-dist)*0.5
            a.x-=nx*overlap*2
            b.x+=nx*overlap*2
            a.y-=ny*overlap
            b.y+=ny*overlap
            // Equal-mass impulse along the collision normal.
            approach:=((b.vx-a.vx)*0.5)*nx+(b.vy-a.vy)*ny
            if approach<0 {
                kick:=math.Min(11,2.2+math.Abs(approach)*1.4)
                a.wobbleVelocity+=kick
                b.wobbleVelocity-=kick
                impulse:=-approach*0.97
                a.vx-=2*impulse*nx
                a.vy-=impulse*ny
                b.vx+=2*impulse*nx
                b.vy+=impulse*ny
            }
        }
    }
    // Clamp after separation so collisions cannot push spheres offscreen.
    for i:=range s.bubbles {
        b:=&s.bubbles[i]
        minX,maxX:=b.radius*2,float64(s.width)-b.radius*2
        minY,maxY:=b.radius,float64(s.height)-b.radius
        b.x=math.Max(minX,math.Min(maxX,b.x))
        b.y=math.Max(minY,math.Min(maxY,b.y))
    }
    // Gradually populate, then replace popped bubbles without surging.
    if s.steps==1 || s.steps%19==0 {
        s.spawn()
    }
}

func (s *bubbleState) advance(steps uint64,width,height int) {
    s.resize(width,height)
    for i:=uint64(0);i<steps;i++ {s.tick()}
}
