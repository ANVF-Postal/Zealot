package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testSharedPreset() ThemePreset {
	return ThemePreset{ID:"bosnia",Name:"Bosnia",Description:"A shared palette",Colors:defaultTheme()}
}

func TestSharedThemeValidationRejectsUnsafeData(t *testing.T) {
	valid := testSharedPreset()
	if err:=validateSharedTheme(&valid);err!=nil {t.Fatal(err)}
	if err:=validateSharedTheme(nil);err==nil {t.Fatal("accepted missing theme")}
	for _, modify := range []func(*ThemePreset){
		func(p *ThemePreset){p.Name="Unsafe\\x00name"},
		func(p *ThemePreset){p.Name="Unsafe\x1b[2J"},
		func(p *ThemePreset){p.Description="Bad\ncontrol"},
		func(p *ThemePreset){p.Description=strings.Repeat("X",maxSharedThemeDescription+1)},
		func(p *ThemePreset){p.Name=strings.Repeat("N",maxSharedThemeName+1)},
		func(p *ThemePreset){p.ID="../outside"},
		func(p *ThemePreset){p.Colors.TextColor="\x1b[31m"},
		func(p *ThemePreset){p.Colors.TextColor=" #FFFFFF"},
	} {
		p:=valid
		modify(&p)
		if err:=validateSharedTheme(&p);err==nil {t.Errorf("accepted unsafe theme: %+v",p)}
	}
}

func TestSharedThemeInstallNeverOverwritesExistingFiles(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME",t.TempDir())
	if err:=ensureThemeLibrary();err!=nil {t.Fatal(err)}
	p:=testSharedPreset()
	first,err:=installSharedTheme(p)
	if err!=nil {t.Fatal(err)}
	if first.FileName!="theme-bosnia.toml" {t.Fatalf("unexpected first filename: %s",first.FileName)}
	firstPath:=filepath.Join(themesDir(),first.FileName)
	original,err:=os.ReadFile(firstPath)
	if err!=nil {t.Fatal(err)}
	second,err:=installSharedTheme(p)
	if err!=nil {t.Fatal(err)}
	if second.FileName!="theme-bosnia1.toml" {t.Fatalf("collision was not suffixed with 1: %s",second.FileName)}
	after,err:=os.ReadFile(firstPath)
	if err!=nil {t.Fatal(err)}
	if !bytes.Equal(original,after) {t.Fatal("second install changed existing theme file")}
	if second.Preset.ID==first.Preset.ID {t.Fatal("installed theme IDs collided")}
	packs,err:=loadThemePacks()
	if err!=nil {t.Fatal(err)}
	if len(packs)<3 {t.Fatalf("installed themes not discoverable: %d packs",len(packs))}
}

func TestThemeShareServerRejectsInvalidAndCopiesSender(t *testing.T) {
	s:=newServer()
	room:=s.room("polls",true)
	var senderOutput,otherOutput bytes.Buffer
	sender:=newPollTestClient("Alice","a",&senderOutput)
	other:=newPollTestClient("Bob","b",&otherOutput)
	room.clients[sender.id]=sender
	room.clients[other.id]=other
	bad:=testSharedPreset()
	bad.Colors.AccentColor="\x1b[9m"
	s.shareTheme("polls",sender,&bad)
	errPackets:=takePollPackets(t,&senderOutput)
	if len(errPackets)!=1 || errPackets[0].Type!="system" {t.Fatalf("invalid theme not rejected: %+v",errPackets)}
	if len(room.messages)!=0 || otherOutput.Len()!=0 {t.Fatal("invalid theme was broadcast or stored")}
	good:=testSharedPreset()
	s.shareTheme("polls",sender,&good)
	delivered:=takePollPackets(t,&otherOutput)
	takePollPackets(t,&senderOutput)
	if len(delivered)!=1 || delivered[0].Type!="theme_result" || delivered[0].Theme==nil ||
		delivered[0].User.Name!="Alice" || delivered[0].Theme.Name!=good.Name || len(room.messages)!=1 {
		t.Fatalf("valid theme was not distributed: %+v",delivered)
	}
	good.Name="Mutated locally"
	if room.messages[0].Theme.Name!="Bosnia" {t.Fatal("server stored sender's mutable theme pointer")}
}

func TestThemeShareRequiresCustomThemeAndConfirmation(t *testing.T) {
	var output bytes.Buffer
	m:=newMessageInteractionTestModel(100)
	m.height=32
	m.connected=true
	m.room="polls"
	m.client=&wireClient{enc:json.NewEncoder(&output)}
	m.themesOpen=true
	m.splashOpen=true
	m.themeEntries=[]themeListEntry{{Preset:defaultThemePack().Themes[0],FileName:defaultThemePackFilename}}
	m.themeSelected=0
	m.beginThemeShare()
	if m.themeShareConfirm || output.Len()!=0 {t.Fatal("built-in theme was shareable")}
	m.themeEntries=[]themeListEntry{{Preset:testSharedPreset(),FileName:"theme-bosnia.toml"}}
	m.beginThemeShare()
	if !m.themeShareConfirm || output.Len()!=0 {t.Fatal("share skipped confirmation or immediately sent")}
	visible:=stripANSISequences(m.renderThemeBrowser())
	if !strings.Contains(visible, "[CONFIRM]") || !strings.Contains(visible,"[CANCEL]") {
		t.Fatalf("confirmation buttons missing: %s",visible)
	}
	m.confirmThemeShare()
	var packet Packet
	if err:=json.NewDecoder(&output).Decode(&packet);err!=nil {t.Fatal(err)}
	if packet.Type!="theme_share" || packet.Theme==nil || packet.Theme.Name!="Bosnia" {
		t.Fatalf("wrong outgoing share request: %+v",packet)
	}
	if m.themesOpen || m.splashOpen || m.themeShareConfirm {t.Fatal("share did not return to chat")}
}

func TestReceivedThemePreviewAndClickableMessage(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME",t.TempDir())
	m:=newMessageInteractionTestModel(95)
	m.height=35
	m.connected=true
	shared:=Packet{ID:"theme-one",Type:"theme_result",User:User{Name:"Alice",Color:"#FFFFFF"},Theme:func()*ThemePreset{p:=testSharedPreset();return &p}()}
	m.appendMessage(shared)
	if !strings.Contains(stripANSISequences(strings.Join(m.renderThemeShareLines(shared,m.width),"\n")),"Alice sent a [Theme]!") {
		t.Fatal("theme message did not render the clickable label")
	}
	rows:=m.renderThemeShareLines(shared,m.width)
	x:=strings.Index(stripANSISequences(rows[0]),"[Theme]")
	if x<0 {t.Fatal("theme link missing")}
	// Normal wide layouts have one outer border column.
	got,ok:=m.sharedThemeLinkAt(1+x,2)
	if !ok || got.ID!=shared.ID {t.Fatalf("theme link failed: %+v, %v",got,ok)}
	m.openReceivedTheme(got)
	if m.themeIncoming==nil || !m.themesOpen || m.themeIncoming.Name!="Bosnia" {
		t.Fatalf("received preview not opened: %+v",m.themeIncoming)
	}
	details:=stripANSISequences(m.renderThemeBrowser())
	for _, label:=range []string{"Bosnia","COLOR PALETTE","[INSTALL]","[CLOSE]"} {
		if !strings.Contains(details,label) {t.Errorf("preview missing %s",label)}
	}
	if strings.Contains(details,"[APPLY]") || strings.Contains(details,"[CLONE]") {
		t.Fatal("incoming theme offered local edit/apply actions")
	}
	m.installReceivedTheme()
	if m.themeIncoming!=nil || m.themesOpen {t.Fatal("install did not return to chat")}
	if _,err:=os.Stat(filepath.Join(themesDir(),"theme-bosnia.toml"));err!=nil {t.Fatal(err)}
}
