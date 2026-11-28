package main

import (
	"testing"
	"time"
	"strings"
)

func TestSettingsTimestampPreferences(t *testing.T) {
	m:=newMessageInteractionTestModel(80)
	stamp:=time.Date(2026,1,2,15,4,0,0,time.Local)
	m.cfg.TimeOffsetMinutes=-90
	if got:=m.timestamp(stamp);got!="[13:34]"{t.Fatalf("offset timestamp: %q",got)}
	m.cfg.TwelveHourTime=true
	if got:=m.timestamp(stamp);got!="[01:34 PM]"{t.Fatalf("12-hour timestamp: %q",got)}
	m.cfg.HideTimestamps=true
	if got:=m.timestampPrefix(stamp);got!=""{t.Fatalf("hidden timestamp: %q",got)}
	p:=interactionTestPacket("hidden","hello")
	for _,width:=range []int{32,80}{
		rows:=m.renderPacketLines(p,width)
		if len(rows)!=m.renderPacketLineCount(p,width){t.Fatalf("width %d: row count mismatch",width)}
		if !strings.Contains(stripANSISequences(strings.Join(rows," ")), "hello"){t.Fatal("missing message body")}
	}
}
