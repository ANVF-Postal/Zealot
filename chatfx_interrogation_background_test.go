package main

import (
    "strconv"
    "strings"
    "testing"
)

// Find the actual background active at a given printable character. The
// original SGR background may be present earlier in the stream, but the
// interrogation renderer must override it with the illuminated value.
func interrogationBackgroundAt(line string, column int) [3]int {
    bg:=[3]int{-1,-1,-1}
    x:=0
    for i:=0;i<len(line); {
        if line[i]==0x1b {
            end:=ansiSequenceEnd(line,i)
            if end<=i {end=i+1}
            if line[end-1]=='m' && strings.HasPrefix(line[i:end],"\x1b[") {
                params:=strings.Split(line[i+2:end-1],";")
                for j:=0;j+4<len(params);j++ {
                    if params[j]=="48" && params[j+1]=="2" {
                        r,e1:=strconv.Atoi(params[j+2])
                        g,e2:=strconv.Atoi(params[j+3])
                        b,e3:=strconv.Atoi(params[j+4])
                        if e1==nil && e2==nil && e3==nil {bg=[3]int{r,g,b}}
                        j+=4
                    }
                }
                if strings.Contains(line[i:end],"[0m") {bg=[3]int{-1,-1,-1}}
            }
            i=end
            continue
        }
        if x==column {return bg}
        x++
        i++
    }
    return bg
}

func TestInterrogationVignettePaintsEmptyChatBackground(t *testing.T) {
    m:=newMessageInteractionTestModel(101)
    m.fxInterrogationAmount=1
    // Completely blank rows should still be shaded from left to right.
    rows:=make([]string,30)
    for i:=range rows {rows[i]=strings.Repeat(" ",chatContentWidth(m.width))}
    m.renderInterrogationRows(rows,0)
    last:=rows[len(rows)-1]
    if strings.TrimSpace(stripANSISequences(last))!="" {t.Fatal("lighting inserted visible characters")}
    xCenter:=chatContentWidth(m.width)/2
    edge:=interrogationBackgroundAt(last,0)
    center:=interrogationBackgroundAt(last,xCenter)
    if edge[0]<0 || center[0]<0 {t.Fatal("blank background left unstyled")}
    if !(edge[0]<center[0] && edge[1]<center[1] && edge[2]<center[2]) {
        t.Fatalf("blank row lacks dark vignette: left=%v centre=%v",edge,center)
    }
    if !strings.Contains(rows[0],"\x1b[48;2;") {
        t.Fatal("upper blank rows have no background effect")
    }
}

func TestInterrogationVignetteShadesCustomMessageBackground(t *testing.T) {
    row:="\x1b[48;2;80;70;60m\x1b[38;2;230;230;230m"+
        strings.Repeat("M",80)
    lit:=interrogationLightRow(row,"#111318","#E6E6E6",80,24,22,1,0,1)
    if stripANSISequences(lit)!=stripANSISequences(row) {
        t.Fatal("message changed during background shading")
    }
    edge:=interrogationBackgroundAt(lit,0)
    center:=interrogationBackgroundAt(lit,39)
    if edge[0]>=80 || center[0]>=80 {t.Fatalf("custom background was not darkened: %v %v",edge,center)}
    if center[0]<=edge[0] {t.Fatalf("custom background has no light pool: %v %v",edge,center)}
}

func TestInterrogationWiderRadiusAndDarkerEdges(t *testing.T) {
    const width,height=100,30
    center:=interrogationLightAt(50,28,width,height,0,1)
    shoulder:=interrogationLightAt(25,22,width,height,0,1)
    edge:=interrogationLightAt(0,22,width,height,0,1)
    corner:=interrogationLightAt(0,0,width,height,0,1)
    if !(center>shoulder && shoulder>edge && edge>corner) {
        t.Fatalf("not a wide, rounded vignette: %f %f %f %f",center,shoulder,edge,corner)
    }
    if shoulder<0.52 {t.Fatalf("light pool is too narrow: shoulder=%f",shoulder)}
    if corner>0.28 {t.Fatalf("corners aren't dark enough: corner=%f",corner)}
}
