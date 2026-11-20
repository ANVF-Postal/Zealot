package main

import (
    "fmt"
    "strconv"
    "strings"
    "time"
)

// Interrogation is a presentation-only ChatFX. No history, message styling,
// scrollbar geometry, or hitboxes are modified. Its opacity persists after
// /stop so the chat can recover across the same five-second transition.
const interrogationFadeDuration = 5*time.Second

func (m *model) advanceInterrogationFade(now time.Time) {
    if now.IsZero() {now=time.Now()}
    if m.fxInterrogationLast.IsZero() {
        m.fxInterrogationLast=now
        return
    }
    delta:=now.Sub(m.fxInterrogationLast)
    m.fxInterrogationLast=now
    if delta<0 {delta=0}
    if delta>interrogationFadeDuration {delta=interrogationFadeDuration}
    change:=delta.Seconds()/interrogationFadeDuration.Seconds()
    if m.currentChatFX().name=="interrogation" {
        m.fxInterrogationAmount=min(1,m.fxInterrogationAmount+change)
    } else {
        m.fxInterrogationAmount=max(0,m.fxInterrogationAmount-change)
        if m.fxInterrogationAmount==0 {m.fxInterrogationLast=time.Time{}}
    }
}

// Map regular, bright, 256-colour, and truecolour SGR foregrounds into RGB
// before dimming them. Background codes and other ANSI styling are retained.
func interrogationANSIColor(parameters string, current [3]int, normal [3]int) [3]int {
    fields:=strings.Split(parameters,";")
    for i:=0;i<len(fields);i++ {
        n,err:=strconv.Atoi(fields[i])
        if err!=nil {continue}
        switch {
        case n==0 || n==39:
            current=normal
        case n==38 && i+4<len(fields) && fields[i+1]=="2":
            r,e1:=strconv.Atoi(fields[i+2])
            g,e2:=strconv.Atoi(fields[i+3])
            b,e3:=strconv.Atoi(fields[i+4])
            if e1==nil && e2==nil && e3==nil {
                current=[3]int{max(0,min(255,r)),max(0,min(255,g)),max(0,min(255,b))}
            }
            i+=4
        case n==38 && i+2<len(fields) && fields[i+1]=="5":
            index,e:=strconv.Atoi(fields[i+2])
            if e==nil && index>=0 && index<=255 {current=interrogationANSI256(index)}
            i+=2
        case n>=30 && n<=37:
            current=interrogationANSI256(n-30)
        case n>=90 && n<=97:
            current=interrogationANSI256(n-90+8)
        }
    }
    return current
}

func interrogationANSI256(index int) [3]int {
    base:=[16][3]int{
        {0,0,0},{187,0,0},{0,187,0},{187,187,0},
        {0,0,187},{187,0,187},{0,187,187},{187,187,187},
        {85,85,85},{255,85,85},{85,255,85},{255,255,85},
        {85,85,255},{255,85,255},{85,255,255},{255,255,255},
    }
    if index<16 {return base[index]}
    if index<232 {
        i:=index-16
        level:=func(v int) int {if v==0 {return 0};return 55+40*v}
        return [3]int{level(i/36),level(i/6%6),level(i%6)}
    }
    gray:=8+(index-232)*10
    return [3]int{gray,gray,gray}
}

func interrogationRGB(hex string, fallback [3]int) [3]int {
    if len(hex)!=7 || hex[0]!='#' {return fallback}
    var v [3]int
    for i:=0;i<3;i++ {
        n,err:=strconv.ParseUint(hex[1+i*2:3+i*2],16,8)
        if err!=nil {return fallback}
        v[i]=int(n)
    }
    return v
}

func interrogationDimRow(row, background, foreground string, amount float64) string {
    if amount<=0 {return row}
    if amount>1 {amount=1}
    bg:=interrogationRGB(background,[3]int{17,19,24})
    normal:=interrogationRGB(foreground,[3]int{230,230,230})
    current:=normal
    // At maximum dimming retain a small ghost of the old messages.
    opacity:=1-0.94*amount
    var output strings.Builder
    output.Grow(len(row)+128)
    emit:=func() {
        red:=int(float64(bg[0])*(1-opacity)+float64(current[0])*opacity)
        green:=int(float64(bg[1])*(1-opacity)+float64(current[1])*opacity)
        blue:=int(float64(bg[2])*(1-opacity)+float64(current[2])*opacity)
        fmt.Fprintf(&output,"\x1b[38;2;%d;%d;%dm",red,green,blue)
    }
    emit()
    for i:=0;i<len(row); {
        if row[i]==0x1b {
            end:=ansiSequenceEnd(row,i)
            if end>i && row[end-1]=='m' && strings.HasPrefix(row[i:end],"\x1b[") {
                current=interrogationANSIColor(row[i+2:end-1],current,normal)
            }
            output.WriteString(row[i:end])
            if end>i && row[end-1]=='m' {emit()}
            i=end
            continue
        }
        output.WriteByte(row[i])
        i++
    }
    output.WriteString("\x1b[39m")
    return output.String()
}

func (m model) interrogationSpotlightIndex() int {
    if len(m.messages)==0 {return -1}
    // A command's system notice must not steal the spotlight from a person.
    for i:=len(m.messages)-1;i>=0;i-- {
        switch m.messages[i].Type {
        case "message","me","whisper_result","letter":
            return i
        }
    }
    return len(m.messages)-1
}

// The point light is anchored to the chat viewport, never a particular
// message row. Incoming messages therefore inherit the existing lighting
// immediately; there is no per-message fade-in stripe as history scrolls.
func (m model) renderInterrogationRows(rows []string, start int) {
    if m.fxDisabled || m.fxInterrogationAmount<=0 || len(rows)==0 {return}
    if fx:=m.currentChatFX().name;fx!="" && fx!="interrogation" {return}
    width:=chatContentWidth(m.width)
    if width<1 {return}
    flicker:=interrogationLampIntensity(m.fxSimSteps,m.fxSalt)
    // The reserved typing row is one more row in the same light field.
    fullHeight:=len(rows)+1
    for i,row:=range rows {
        rows[i]=interrogationLightRow(
            row,m.cfg.Theme.Background,m.cfg.Theme.TextColor,
            width,fullHeight,i,m.fxInterrogationAmount,0,flicker,
        )
    }
}

func (m model) renderInterrogationTypingRow(row string, y int) string {
    if m.fxDisabled || m.fxInterrogationAmount<=0 {return row}
    if fx:=m.currentChatFX().name;fx!="" && fx!="interrogation" {return row}
    return interrogationLightRow(
        row,m.cfg.Theme.Background,m.cfg.Theme.TextColor,
        chatContentWidth(m.width),y+1,y,m.fxInterrogationAmount,0,
        interrogationLampIntensity(m.fxSimSteps,m.fxSalt),
    )
}

func (m model) interrogationMessageRange(index int) messageRowRange {
    if index<0 || index>=len(m.messages) {return messageRowRange{Start:-1,End:-1}}
    if len(m.messageRowRanges)==len(m.messages) {
        return m.messageRowRanges[index]
    }
    row:=0
    for i,msg:=range m.messages {
        end:=row+max(1,m.renderPacketLineCount(msg,m.width))
        if i==index {return messageRowRange{Start:row,End:end}}
        row=end
    }
    return messageRowRange{Start:-1,End:-1}
}
