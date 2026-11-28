package main

import "time"

// Offset is an adjustment to the existing client-local display time.
func (m model) timestampText(t time.Time) string {
	if m.cfg.HideTimestamps {return ""}
	t=t.Local().Add(time.Duration(m.cfg.TimeOffsetMinutes)*time.Minute)
	if m.cfg.TwelveHourTime{return t.Format("03:04 PM")}
	return t.Format("15:04")
}

func (m model) timestamp(t time.Time) string {
	if m.cfg.HideTimestamps{return ""}
	return "["+m.timestampText(t)+"]"
}

func (m model) timestampPrefix(t time.Time) string {
	if m.cfg.HideTimestamps{return ""}
	return m.timestamp(t)+" "
}

func compactTimestamp(text string) string {
	if text==""{return ""}
	return "["+text+"]"
}

func compactTimestampGap(stamp string) int {
	if stamp==""{return 0}
	return 1
}
