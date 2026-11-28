package main

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

var optionsCategories = []string{"General", "Chat", "Composer", "ChatFX", "Advanced"}

type optionsRow struct {
	label string
	value string
	change func(int)
	action func()
}

func (m model) splashOptionsButtonContains(x, y int) bool {
	if !m.splashOpen || m.themesOpen || m.commandsOpen || m.optionsOpen {
		return false
	}
	button := "[SETTINGS]"
	left := m.themeBrowserContentStartX()
	start := left + max(0, (chatContentWidth(m.width)-lipgloss.Width(button))/2)
	return y == 2+len(m.splashRows())-1-m.splashScroll &&
		y >= 2 && y < 2+m.messageHeight() && x >= start && x < start+lipgloss.Width(button)
}

func (m *model) openOptionsBrowser() {
	m.optionsOpen = true
	m.splashOpen = true
	m.usersOpen = false
	m.optionsCategory, m.optionsSelected, m.optionsScroll = 0, 0, 0
	m.optionsDraft = m.cfg
	m.optionsDraft.FXDisabled=m.fxDisabled
	m.optionsDraft.FXFPS=m.fxRenderFPS
	if m.optionsDraft.FXFPS<1 {m.optionsDraft.FXFPS=chatFXDefaultFPS}
	m.optionsStatus = ""
}

func (m *model) closeOptionsBrowser() {
	if m.optionsDraft != m.cfg && m.optionsStatus != "Unsaved changes. Esc again to discard." {
		m.optionsStatus = "Unsaved changes. Esc again to discard."
		return
	}
	m.optionsOpen = false
	m.optionsStatus = ""
	m.clampSplashScroll()
}

func optionsToggle(v bool) string {
	if v { return "[ ON ]" }
	return "[ OFF ]"
}

func (m *model) optionsRows() []optionsRow {
	d := &m.optionsDraft
	var rows []optionsRow
	switch m.optionsCategory {
	case 0:
		rows = []optionsRow{
			{label:"Username (use /rename)", value:d.Name},
			{label:"Color (use /color)", value:d.UserColor},
			{label:"Terminal bell",value:optionsBellLabel(*d),change:func(n int){mode:=0;if d.Bell{mode=1};if d.BellMention{mode=2};mode=(mode+n+3)%3;d.Bell=mode!=0;d.BellMention=mode==2}},
			{label:"Splash on startup",value:optionsToggle(d.ShowStartupSplash),change:func(_ int){d.ShowStartupSplash=!d.ShowStartupSplash}},
			{label:"Random splash logo",value:optionsToggle(!d.FixedSplashLogo),change:func(_ int){d.FixedSplashLogo=!d.FixedSplashLogo}},
		}
	case 1:
		format:="24-hour";if d.TwelveHourTime{format="12-hour"}
		sign:="+";offset:=d.TimeOffsetMinutes;if offset<0{sign="-";offset=-offset}
		rows=[]optionsRow{
			{label:"Show timestamps",value:optionsToggle(!d.HideTimestamps),change:func(_ int){d.HideTimestamps=!d.HideTimestamps}},
			{label:"Timestamp format",value:"[ "+format+" ]",change:func(_ int){d.TwelveHourTime=!d.TwelveHourTime}},
			{label:"Offset hours",value:fmt.Sprintf("[-] %s%02d [+]",sign,offset/60),change:func(n int){d.TimeOffsetMinutes=min(1439,max(-1439,d.TimeOffsetMinutes+n*60))}},
			{label:"Offset minutes",value:fmt.Sprintf("[-] %s%02d [+]",sign,offset%60),change:func(n int){d.TimeOffsetMinutes=min(1439,max(-1439,d.TimeOffsetMinutes+n))}},
			{label:"Highlight mentions",value:optionsToggle(!d.DisableMentionHighlight),change:func(_ int){d.DisableMentionHighlight=!d.DisableMentionHighlight}},
			{label:"Confirm Quit click",value:optionsToggle(!d.QuitWithoutConfirm),change:func(_ int){d.QuitWithoutConfirm=!d.QuitWithoutConfirm}},
		}
	case 2:
		rows = []optionsRow{
			{label:"Input height", value:fmt.Sprintf("[-] %d [+]",d.InputHeight), change:func(n int){d.InputHeight = min(8,max(1,d.InputHeight+n))}},
			{label:"Adaptive height", value:optionsToggle(d.AdaptiveHeight), change:func(_ int){d.AdaptiveHeight = !d.AdaptiveHeight}},
			{label:"Maximum height (%)", value:fmt.Sprintf("[-] %d [+]",d.HeightMaxPercent), change:func(n int){if d.AdaptiveHeight {d.HeightMaxPercent=min(100,max(1,d.HeightMaxPercent+n))}}},
		}
	case 3:
		rows = []optionsRow{{label:"Enable ChatFX",value:optionsToggle(!d.FXDisabled),change:func(_ int){d.FXDisabled=!d.FXDisabled}},{label:"Animation FPS",value:fmt.Sprintf("[-] %d [+]",d.FXFPS),change:func(n int){d.FXFPS=min(chatFXMaxFPS,max(1,d.FXFPS+n))}}}
		for _,name:=range optionsEffects {bit:=optionsEffectBit(name);value:="[x]";if d.FXBlocked&bit!=0{value="[ ]"};rows=append(rows,optionsRow{label:name,value:value,change:func(_ int){d.FXBlocked^=bit}})}
	case 4:
		rows = []optionsRow{{label:"Configuration path", value:configPath()}}
	}
	return rows
}

func (m *model) optionsActions() []optionsRow {
	d := &m.optionsDraft
	return []optionsRow{{label:"[Save]",action:m.saveOptions},optionsRow{label:"[Cancel]",action:func(){m.optionsDraft=m.cfg;m.closeOptionsBrowser()}},optionsRow{label:"[Defaults]",action:func(){
		defaults:=defaultConfig()
		d.Bell=defaults.Bell
		d.BellMention=false
		d.HideTimestamps=false
		d.TwelveHourTime=false
		d.TimeOffsetMinutes=0
		d.DisableMentionHighlight=false
		d.QuitWithoutConfirm=false
		d.ShowStartupSplash=defaults.ShowStartupSplash
		d.FixedSplashLogo=false
		d.FXDisabled=false
		d.FXFPS=chatFXDefaultFPS
		d.FXBlocked=0
		d.InputHeight=defaults.InputHeight
		d.AdaptiveHeight=defaults.AdaptiveHeight
		d.HeightMaxPercent=defaults.HeightMaxPercent
		m.optionsStatus="Defaults staged; save to apply."
	}}}
}

func (m *model) saveOptions() {
	d:=m.optionsDraft
	var entries []configFileEntry
	old:=configFileValues(m.cfg)
	for i,entry:=range configFileValues(d){if entry.value!=old[i].value{entries=append(entries,entry)}}
	if len(entries)>0 {if err:=saveConfigValues(entries);err!=nil{m.optionsStatus="Save failed: "+err.Error();return}}
	m.cfg=d
	m.fxDisabled=d.FXDisabled
	m.fxRenderFPS=d.FXFPS
	m.fxFading=nil
	m.fxTickGeneration++
	m.fxTickPending=false
	if !m.terminalEnabled(){m.terminalClearReveals()}
	m.resizeInput()
	m.rebuildMessageCache()
	m.clampScroll()
	m.clampSplashScroll()
	m.optionsDraft=m.cfg
	m.optionsStatus="Saved."
}

func (m model) optionsVisibleRows() int { return max(1,m.messageHeight()-5) }

func (m *model) clampOptionsScroll() {
	m.optionsScroll=min(max(0,m.optionsScroll),max(0,len(m.optionsRows())-m.optionsVisibleRows()))
}

func (m *model) selectOptionsCategory(n int) {
	m.optionsCategory=min(len(optionsCategories)-1,max(0,n))
	m.optionsSelected,m.optionsScroll=0,0
	m.optionsStatus=""
}

func (m *model) optionsKey(key string) {
	if key=="esc" || key=="escape" {m.closeOptionsBrowser();return}
	m.optionsStatus=""
	rows:=append(m.optionsRows(),m.optionsActions()...)
	switch key {
	case "tab": m.selectOptionsCategory((m.optionsCategory+1)%len(optionsCategories));return
	case "shift+tab": m.selectOptionsCategory((m.optionsCategory+len(optionsCategories)-1)%len(optionsCategories));return
	case "up": m.optionsSelected=max(0,m.optionsSelected-1)
	case "down": m.optionsSelected=min(len(rows)-1,m.optionsSelected+1)
	case "pgup": m.optionsSelected=max(0,m.optionsSelected-m.optionsVisibleRows())
	case "pgdown": m.optionsSelected=min(len(rows)-1,m.optionsSelected+m.optionsVisibleRows())
	case "ctrl+s": m.saveOptions()
	case "left","right","enter","space":
		row:=rows[m.optionsSelected]
		if row.action!=nil && (key=="enter" || key=="space"){row.action();return}
		if row.change!=nil {delta:=1;if key=="left"{delta=-1};row.change(delta)}
	}
	visible:=m.optionsVisibleRows()
	if m.optionsSelected>=len(m.optionsRows()) {return}
	if m.optionsSelected<m.optionsScroll {m.optionsScroll=m.optionsSelected}
	if m.optionsSelected>=m.optionsScroll+visible {m.optionsScroll=m.optionsSelected-visible+1}
	m.clampOptionsScroll()
}

func (m *model) optionsClick(x,y int) {
	if y==0 {if x<lipgloss.Width("ZEALOT") || x>=m.width-lipgloss.Width("[X]"){m.closeOptionsBrowser()};return}
	left,right:=m.themeBrowserColumnWidths()
	x-=m.themeBrowserContentStartX()
	if x<left {
		if index:=y-4; index>=0 && index<len(optionsCategories){m.selectOptionsCategory(index)}
		return
	}
	if x<=left || y<4{return}
	rows:=m.optionsRows()
	if y==1+m.messageHeight() {
		for i,a:=range m.optionsActions(){start:=optionsActionStart(i); if x-left-1>=start && x-left-1<start+lipgloss.Width(a.label){a.action();return}}
		return
	}
	if y>=m.messageHeight()-1{return}
	i:=y-4+m.optionsScroll
	if i<0 || i>=len(rows){return}
	m.optionsSelected=i
	m.optionsStatus=""
	row:=rows[i]
	if row.action!=nil {row.action();return}
	if row.change!=nil {
		delta:=1
		if strings.HasPrefix(row.value,"[-]") && x-left-1<right-1-lipgloss.Width(row.value)+3 {delta=-1}
		row.change(delta)
	}
}

func optionsActionStart(i int) int {return []int{0,7,16}[i]}

func (m model) renderOptionsBrowser() string {
	left,right:=m.themeBrowserColumnWidths()
	height:=max(1,m.messageHeight())
	bg:=lipgloss.Color(m.cfg.Theme.Background)
	accent:=lipgloss.Color(m.cfg.Theme.AccentColor)
	border:=lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.BorderColor)).Background(bg)
	normal:=lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.TextColor)).Background(bg)
	selected:=lipgloss.NewStyle().Foreground(bg).Background(accent)
	l:=[]string{m.styles.accent.Render("SETTINGS"),border.Render(strings.Repeat("─",left))}
	for i,label:=range optionsCategories {
		if i==m.optionsCategory {l=append(l,selected.Render("› "+label))}else{l=append(l,normal.Render("  "+label))}
	}
	l=fitThemeRows(l,height,left,bg)
	status:=""
	if m.optionsDraft!=m.cfg {status="Unsaved • Ctrl+S"}
	if m.optionsStatus!="" {status=m.optionsStatus}
	r:=[]string{m.styles.accent.Render(optionsCategories[m.optionsCategory]),border.Render(strings.Repeat("─",right))}
	rows:=m.optionsRows()
	visible:=m.optionsVisibleRows()
	scroll:=min(max(0,m.optionsScroll),max(0,len(rows)-visible))
	top,thumb,track:=scrollbarGeometry(len(rows),visible,visible,scroll)
	for j:=0;j<visible && j+2<height;j++ {
		i:=scroll+j
		line:=""
		if i<len(rows){
			row:=rows[i]
			width:=max(1,right-1)
			gap:=max(1,width-lipgloss.Width(row.label)-lipgloss.Width(row.value))
			line=row.label+strings.Repeat(" ",gap)+row.value
			if i==m.optionsSelected {line=normal.Bold(true).Render(truncateANSI("› "+row.label,max(0,width-lipgloss.Width(row.value)-1)))+normal.Render(strings.Repeat(" ",max(0,width-lipgloss.Width(truncateANSI("› "+row.label,max(0,width-lipgloss.Width(row.value)-1)))-lipgloss.Width(row.value))))+optionsValueStyle(normal,accent,m.cfg.Theme.MutedTextColor,row.value).Render(row.value)
			}else{line=normal.Render(truncateANSI(row.label+strings.Repeat(" ",gap),max(0,width-lipgloss.Width(row.value))))+optionsValueStyle(normal,accent,m.cfg.Theme.MutedTextColor,row.value).Render(row.value)}
		}
		glyph:="│"
		if len(rows)>visible && j<track && j>=top && j<top+thumb {glyph="█"}
		r=append(r,fitThemeRowWithBackground(line,max(0,right-1),bg)+border.Render(glyph))
	}
	if height>=5 {
		r=fitThemeRows(r,height-3,right,bg)
		r=append(r,normal.Render(status),border.Render(strings.Repeat("─",right)))
		buttons:=""
		for i,a:=range m.optionsActions(){if i>0 {buttons+=" "};if m.optionsSelected==len(rows)+i{buttons+=selected.Render(a.label)}else{buttons+=m.styles.accent.Render(a.label)}}
		r=append(r,buttons)
	}
	r=fitThemeRows(r,height,right,bg)
	div:=make([]string,height)
	for i:=range div{div[i]=border.Render("│")}
	content:=lipgloss.JoinHorizontal(lipgloss.Top,strings.Join(l,"\n"),strings.Join(div,"\n"),strings.Join(r,"\n"))
	return m.chatBorderStyle().Width(m.width).Height(m.messageHeight()).Render(content)
}

var optionsEffects=[]string{"rain","snow","blizzard","embers","fire","matrix","confetti","fireworks","static","glitch","interrogation","bubbles","aquarium","terminal"}
func optionsEffectBit(name string) uint16 {
	for i,v:=range optionsEffects{if v==name{return 1<<i}}
	return 0
}
func optionsBlockedEffects(mask uint16) string {
	var names []string
	for _,name:=range optionsEffects{if mask&optionsEffectBit(name)!=0{names=append(names,name)}}
	return strings.Join(names,",")
}
func optionsBellValue(c Config) string {
	if c.BellMention{return strconv.Quote("mention")}
	return strconv.FormatBool(c.Bell)
}
func optionsBellLabel(c Config) string {
	if c.BellMention{return "[ MENTION ]"}
	return optionsToggle(c.Bell)
}
func (m model) shouldRingBell(p Packet) bool {
	if !m.cfg.Bell || strings.EqualFold(p.User.Name,m.user.Name){return false}
	if !m.cfg.BellMention{return p.Type=="message"}
	if p.Type=="whisper_result" && strings.EqualFold(p.WhisperTo,m.user.Name){return true}
	if p.Type!="message"{return false}
	for _,name:=range p.Mentions{if strings.EqualFold(name,m.user.Name){return true}}
	if p.ReplyTo!=""{for _,parent:=range m.messages{if parent.ID==p.ReplyTo && strings.EqualFold(parent.User.Name,m.user.Name){return true}}}
	return false
}

func optionsValueStyle(normal lipgloss.Style, accent interface{RGBA()(uint32,uint32,uint32,uint32)}, muted string,value string) lipgloss.Style {
	switch value {
	case "[ ON ]","[ MENTION ]","[x]":return normal.Foreground(accent).Bold(true)
	case "[ OFF ]","[ ]":return normal.Foreground(lipgloss.Color(muted))
	default:return normal
	}
}
