package main

import "testing"

func TestOptionsComposerDraftAndSave(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME",t.TempDir())
	m:=newModel(defaultConfig(),"default",false,nil)
	m.width,m.height=80,30
	m.openOptionsBrowser()
	m.selectOptionsCategory(2)
	m.optionsKey("right")
	if m.optionsDraft.InputHeight!=2 || m.cfg.InputHeight!=1 {t.Fatal("composer change was not staged")}
	m.optionsKey("ctrl+s")
	if m.cfg.InputHeight!=2 {t.Fatal("composer change was not applied")}
	cfg,err:=loadConfig()
	if err!=nil || cfg.InputHeight!=2 {t.Fatalf("composer change was not persisted: %v",err)}
}

func TestOptionsDiscard(t *testing.T) {
	m:=newModel(defaultConfig(),"default",false,nil)
	m.width,m.height=80,30
	m.openOptionsBrowser()
	m.selectOptionsCategory(2)
	m.optionsKey("right")
	m.optionsKey("esc")
	if !m.optionsOpen {t.Fatal("dirty options closed without confirmation")}
	m.optionsKey("esc")
	if m.optionsOpen || m.cfg.InputHeight!=1 {t.Fatal("discard did not preserve configuration")}
}
