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
    rim [24]float64 // independent radial soft-body displacement at each border node
    rimSpeed [24]float64 // spring velocity for each border node
}

type bubblePop struct {
    x,y float64
    age float64
    color [3]int
    fragments [12]bubbleFragment
}

type bubbleFragment struct {
    x,y,vx,vy float64
    glyph rune
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
        for i:=range p.fragments {
            part:=&p.fragments[i]
            part.x+=part.vx*dt
            part.y+=part.vy*dt
            part.vx*=0.975
            part.vy=part.vy*0.975+2.5*dt
        }
        if p.age<0.65 {pops=append(pops,p)}
    }
    s.pops=pops
    alive:=s.bubbles[:0]
    for _,bubble:=range s.bubbles {
        bubble.age+=dt
        if bubble.age>=bubble.lifetime {
            if len(s.pops)<bubbleMaximum {
                s.pops=append(s.pops,newBubblePop(bubble,s.salt+s.steps))
            }
            continue // immediate pop: the sphere is gone this frame
        }
        // Underdamped spring: squash, overshoot, then settle.
        bubble.advanceRim(dt)
        bubble.x+=bubble.vx*dt
        bubble.y+=bubble.vy*dt
        // Permit the centre to approach a boundary more closely than the
        // resting radius; the border nodes, rather than a rigid circle, then
        // flatten against the actual wall.
        minX,maxX:=bubble.radius*2*0.78,float64(s.width)-bubble.radius*2*0.78
        minY,maxY:=bubble.radius*0.78,float64(s.height)-bubble.radius*0.78
        if bubble.x<minX {bubble.x=minX;bubble.vx=math.Abs(bubble.vx);bubble.pressRim(math.Pi,0.32+math.Min(0.3,math.Abs(bubble.vx)*0.025))}
        if bubble.x>maxX {bubble.x=maxX;bubble.vx=-math.Abs(bubble.vx);bubble.pressRim(0,0.32+math.Min(0.3,math.Abs(bubble.vx)*0.025))}
        if bubble.y<minY {bubble.y=minY;bubble.vy=math.Abs(bubble.vy);bubble.pressRim(-math.Pi/2,0.30+math.Min(0.3,math.Abs(bubble.vy)*0.04))}
        if bubble.y>maxY {bubble.y=maxY;bubble.vy=-math.Abs(bubble.vy);bubble.pressRim(math.Pi/2,0.30+math.Min(0.3,math.Abs(bubble.vy)*0.04))}
        bubble.constrainRimToWalls(s.width,s.height)
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
            angle:=math.Atan2(dy,dx)
            required:=a.rimRadius(angle)+b.rimRadius(angle+math.Pi)
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
                a.pressRim(math.Atan2(ny,nx),0.20+0.035*kick)
                b.pressRim(math.Atan2(-ny,-nx),0.20+0.035*kick)
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
        minX,maxX:=b.radius*2*0.78,float64(s.width)-b.radius*2*0.78
        minY,maxY:=b.radius*0.78,float64(s.height)-b.radius*0.78
        b.x=math.Max(minX,math.Min(maxX,b.x))
        b.y=math.Max(minY,math.Min(maxY,b.y))
        b.constrainRimToWalls(s.width,s.height)
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

// The soft body is 24 independent radial masses joined in a cyclic ring.
// Springs to the rest circle and neighboring masses transmit an impact
// around its perimeter, rather than squashing the entire bubble uniformly.
func (b *bubbleParticle) advanceRim(dt float64) {
    var forces [24]float64
    for i:=range b.rim {
        left:=(i+len(b.rim)-1)%len(b.rim)
        right:=(i+1)%len(b.rim)
        forces[i]=-47*b.rim[i]+26*(b.rim[left]+b.rim[right]-2*b.rim[i])-10*b.rimSpeed[i]
    }
    for i:=range b.rim {
        b.rimSpeed[i]+=forces[i]*dt
        b.rim[i]+=b.rimSpeed[i]*dt
        b.rim[i]=math.Max(-0.48,math.Min(0.25,b.rim[i]))
    }
}

// Angle is in physically round coordinates (x is half terminal width).
// Contact compresses the nodes nearest the contact normal. A smooth bell
// profile leaves the opposite side largely unchanged.
func (b *bubbleParticle) pressRim(angle,depth float64) {
    for i:=range b.rim {
        theta:=2*math.Pi*float64(i)/float64(len(b.rim))
        delta:=math.Atan2(math.Sin(theta-angle),math.Cos(theta-angle))
        strength:=math.Exp(-delta*delta/0.22)
        b.rimSpeed[i]-=depth*strength*22
    }
}

func (b bubbleParticle) rimRadius(angle float64) float64 {
    phase:=math.Mod(angle/(2*math.Pi)+1,1)*float64(len(b.rim))
    i:=int(math.Floor(phase))
    t:=phase-float64(i)
    displacement:=b.rim[i]*(1-t)+b.rim[(i+1)%len(b.rim)]*t
    return b.radius*(1+displacement)
}

func newBubblePop(b bubbleParticle,seed uint64) bubblePop {
    pop:=bubblePop{x:b.x,y:b.y,color:b.color}
    symbols:=[]rune{'*','+','·','✦','.'}
    for i:=range pop.fragments {
        h:=chatFXHash(seed+uint64(i)*0x9e3779b97f4a7c15)
        angle:=2*math.Pi*(float64(i)+0.5*chatFXUnit(h))/float64(len(pop.fragments))
        speed:=3+7*chatFXUnit(h+1)
        distance:=b.radius*(0.45+0.55*chatFXUnit(h+2))
        pop.fragments[i]=bubbleFragment{
            x:b.x+2*distance*math.Cos(angle),
            y:b.y+distance*math.Sin(angle),
            vx:b.vx*0.20+2*speed*math.Cos(angle),
            vy:b.vy*0.20+speed*math.Sin(angle),
            glyph:symbols[int(chatFXHash(h+3)%uint64(len(symbols)))],
        }
    }
    return pop
}

// Each perimeter mass is projected against all four real wall planes.
// Unlike a rigid-circle collision, only the nodes contacting the wall flatten
// while tension carries the impact into their neighbors over later steps.
func (b *bubbleParticle) constrainRimToWalls(width,height int) {
    if b.radius<=0 {return}
    for i:=range b.rim {
        theta:=2*math.Pi*float64(i)/float64(len(b.rim))
        c,si:=math.Cos(theta),math.Sin(theta)
        allowed:=b.radius*1.25
        if c< -0.0001 {allowed=math.Min(allowed,b.x/(-2*c))}
        if c>0.0001 {allowed=math.Min(allowed,(float64(width)-b.x)/(2*c))}
        if si< -0.0001 {allowed=math.Min(allowed,b.y/(-si))}
        if si>0.0001 {allowed=math.Min(allowed,(float64(height)-b.y)/si)}
        limit:=math.Max(-0.48,allowed/b.radius-1)
        if b.rim[i]>limit {
            b.rim[i]=limit
            if b.rimSpeed[i]>0 {b.rimSpeed[i]*=-0.25}
        }
    }
}
