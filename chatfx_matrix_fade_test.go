package main

import (
    "reflect"
    "strconv"
    "testing"
)

func TestMatrixFadeGradientReachesThemeBackground(t *testing.T) {
    for _,background:=range []string{"#111318","#EDE8DF","#000000","#293C56"} {
        colors:=matrixFadePalette(background)
        if len(colors)!=matrixFadeSteps {t.Fatalf("gradient length %d",len(colors))}
        if colors[0]!="#24DA63" {t.Fatalf("full intensity should be Matrix green, got %s",colors[0])}
        distinct:=map[string]bool{}
        prevDistance:=int(^uint(0)>>1)
        for _,color:=range colors {
            distinct[color]=true
            if len(color)!=7 || color[0]!='#' {t.Fatalf("invalid shade %q",color)}
            rgb:=func(s string) [3]int {
                var out [3]int
                for i:=0;i<3;i++ {
                    n,err:=strconv.ParseUint(s[1+i*2:3+i*2],16,8)
                    if err!=nil {t.Fatal(err)}
                    out[i]=int(n)
                }
                return out
            }
            c,b:=rgb(color),rgb(background)
            distance:=0
            for i:=0;i<3;i++ {d:=c[i]-b[i];if d<0 {d=-d};distance+=d}
            if distance>prevDistance {t.Fatalf("Matrix trail brightened: %s against %s",color,background)}
            prevDistance=distance
        }
        if len(distinct)<25 {t.Fatalf("Matrix fade still has too few shades: %d",len(distinct))}
        if prevDistance>20 {t.Fatalf("Matrix tail did not fade near background: distance=%d",prevDistance)}
    }
}

func TestMatrixPositionsFadeWithBrightHeadsAndTheme(t *testing.T) {
    m:=newMessageInteractionTestModel(120)
    m.fxSalt=71
    m.cfg.Theme.Background="#111318"
    m.fxSimSteps=135
    first:=m.matrixPositions(120,40)
    seen:=map[string]bool{}
    bright:=false
    for _,row:=range first {
        for _,cell:=range row {
            seen[cell.color]=true
            if cell.color=="#D7FFDE" {bright=true}
        }
    }
    if !bright || len(seen)<12 {t.Fatalf("Matrix gradient missing: bright=%v colors=%d",bright,len(seen))}
    m.fxRenderFPS=1
    one:=m.matrixPositions(120,40)
    m.fxRenderFPS=60
    if !reflect.DeepEqual(one,m.matrixPositions(120,40)) {
        t.Fatal("Matrix colours or positions changed with render FPS")
    }
    m.cfg.Theme.Background="#F0F0F0"
    lighter:=m.matrixPositions(120,40)
    if reflect.DeepEqual(first,lighter) {
        t.Fatal("Matrix trail colours did not respond to theme background")
    }
}

func BenchmarkMatrixFadePositions(b *testing.B) {
    m:=newMessageInteractionTestModel(160)
    m.cfg.Theme.Background="#111318"
    m.fxSalt=71
    b.ReportAllocs()
    for i:=0;i<b.N;i++ {
        m.fxSimSteps=uint64(i*2)
        m.matrixPositions(160,45)
    }
}
