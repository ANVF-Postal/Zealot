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
        duration:=glitchErrorDuration(seed)
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
        cursor.paused+=duration
    }
    cursor.sample=glitchScheduleSample{adjustedTick:tick-cursor.paused}
    cursor.tick=tick
    cursor.valid=true
    glitchSchedules.cursors[salt]=cursor
    return cursor.sample
}
