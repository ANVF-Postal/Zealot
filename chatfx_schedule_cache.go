package main

import "sync"

// These cursors memoize deterministic event schedules. They are process-local
// optimizations, not animation state: backward seeks and salt changes still
// produce the same frames as computing the schedule from tick zero.
type interferenceScheduleKey struct {
    salt uint64
    glitch bool
}
type interferenceScheduleCursor struct {
    tick uint64
    index uint64
    next uint64 // end of the preceding burst
    seed uint64
    active bool
    valid bool
}
var interferenceSchedules = struct {
    sync.Mutex
    cursors map[interferenceScheduleKey]interferenceScheduleCursor
}{cursors:make(map[interferenceScheduleKey]interferenceScheduleCursor)}

func cachedInterferenceBurst(tick,salt uint64,glitch bool) (uint64,bool) {
    key:=interferenceScheduleKey{salt,glitch}
    interferenceSchedules.Lock()
    defer interferenceSchedules.Unlock()
    if _,ok:=interferenceSchedules.cursors[key];!ok && len(interferenceSchedules.cursors)>=32 { clear(interferenceSchedules.cursors) }
    cursor:=interferenceSchedules.cursors[key]
    if cursor.valid && tick==cursor.tick {return cursor.seed,cursor.active}
    if !cursor.valid || tick<cursor.tick {
        cursor=interferenceScheduleCursor{}
    }
    for event:=cursor.index;event<100000;event++ {
        seed:=chatFXHash(salt+(event+1)*0x9e3779b97f4a7c15)
        start:=cursor.next+interferenceGap(seed,glitch)
        length:=uint64(3+(seed>>24)%10)
        if glitch {length=uint64(4+(seed>>24)%13)}
        if tick<start || tick<start+length {
            cursor.index=event
            cursor.tick=tick
            cursor.seed=seed
            cursor.active=tick>=start
            cursor.valid=true
            interferenceSchedules.cursors[key]=cursor
            return cursor.seed,cursor.active
        }
        cursor.next=start+length
        cursor.index=event+1
    }
    cursor.tick=tick
    cursor.seed=0
    cursor.active=false
    cursor.valid=true
    interferenceSchedules.cursors[key]=cursor
    return 0,false
}

type glitchScheduleSample struct {
    event glitchErrorEvent
    active bool
    adjustedTick uint64
}
type glitchScheduleCursor struct {
    tick uint64
    index uint64
    next uint64 // end of the preceding diagnostic
    paused uint64 // durations of completed diagnostics
    duration uint64 // duration of the current event (computed once)
    sample glitchScheduleSample
    valid bool
}
var glitchSchedules = struct {
    sync.Mutex
    cursors map[uint64]glitchScheduleCursor
}{cursors:make(map[uint64]glitchScheduleCursor)}

func cachedGlitchSchedule(tick,salt uint64) glitchScheduleSample {
    glitchSchedules.Lock()
    defer glitchSchedules.Unlock()
    if _,ok:=glitchSchedules.cursors[salt];!ok && len(glitchSchedules.cursors)>=32 { clear(glitchSchedules.cursors) }
    cursor:=glitchSchedules.cursors[salt]
    if cursor.valid && tick==cursor.tick {return cursor.sample}
    if !cursor.valid || tick<cursor.tick {
        cursor=glitchScheduleCursor{}
    }
    for event:=cursor.index;event<100000;event++ {
        seed:=chatFXHash(salt^0x7e827aa2+(event+1)*0x9e3779b97f4a7c15)
        start:=cursor.next+270+(seed>>8)%720
        if tick<start {
            cursor.sample=glitchScheduleSample{adjustedTick:tick-cursor.paused}
            cursor.tick=tick
            cursor.valid=true
            glitchSchedules.cursors[salt]=cursor
            return cursor.sample
        }
        duration:=cursor.duration
        if duration==0 {
            duration=glitchErrorDuration(seed)
            cursor.duration=duration
        }
        if tick<start+duration {
            cursor.sample=glitchScheduleSample{
                event:glitchErrorEvent{start:start,duration:duration,seed:seed},
                active:true,
                adjustedTick:tick-cursor.paused-(tick-start),
            }
            cursor.tick=tick
            cursor.valid=true
            glitchSchedules.cursors[salt]=cursor
            return cursor.sample
        }
        cursor.next=start+duration
        cursor.index=event+1
        cursor.duration=0
        cursor.paused+=duration
    }
    cursor.sample=glitchScheduleSample{adjustedTick:tick-cursor.paused}
    cursor.tick=tick
    cursor.valid=true
    glitchSchedules.cursors[salt]=cursor
    return cursor.sample
}


// Minor Glitch disturbances follow a deterministic renewal schedule rather
// than a repeated fixed-width pulse. A cursor makes sequential 30 Hz frames
// constant-time while backward seeks recompute the original timeline.
type minorGlitchCursor struct {
    tick, next, index uint64
    start, duration uint64
    seed uint64
    valid bool
}
var minorGlitchSchedules = struct {
    sync.Mutex
    cursors map[uint64]minorGlitchCursor
}{cursors:make(map[uint64]minorGlitchCursor)}

func minorGlitchPulse(tick,salt uint64) float64 {
    minorGlitchSchedules.Lock()
    defer minorGlitchSchedules.Unlock()
    if _,exists:=minorGlitchSchedules.cursors[salt];!exists && len(minorGlitchSchedules.cursors)>=32 {
        clear(minorGlitchSchedules.cursors)
    }
    c:=minorGlitchSchedules.cursors[salt]
    if !c.valid || tick<c.tick {c=minorGlitchCursor{}}
    // Consecutive periods have independent duration and quiet-time samples.
    // Starts are typically separated by 1–8 seconds, not a metronome.
    for c.index<100000 {
        seed:=chatFXHash(salt+(c.index+1)*0xd1342543de82ef95)
        gap:=uint64(18+(seed>>9)%171) // 0.6–6.3 seconds of quiet
        duration:=uint64(14+(seed>>24)%91) // 0.47–3.47 seconds active
        start:=c.next+gap
        if tick<start {
            c.tick=tick
            c.valid=true
            minorGlitchSchedules.cursors[salt]=c
            return 0
        }
        if tick<start+duration {
            // Asymmetric attack and release give some faults a sharp onset,
            // and others a slow creep, without introducing a regular cycle.
            phase:=float64(tick-start)/float64(duration)
            attack:=0.16+0.34*chatFXUnit(seed+3)
            intensity:=0.0
            if phase<attack {
                intensity=phase/attack
            } else {
                intensity=(1-phase)/(1-attack)
            }
            c.tick=tick
            c.start=start
            c.duration=duration
            c.seed=seed
            c.valid=true
            minorGlitchSchedules.cursors[salt]=c
            return intensity
        }
        c.index++
        c.next=start+duration
    }
    return 0
}
