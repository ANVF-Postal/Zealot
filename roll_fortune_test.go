package main

import (
	"bufio"
	"encoding/json"
	"net"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

func TestDiceNotationValidationAndLimits(t *testing.T) {
	for _, tc := range []struct { input string; dice, sides int }{
		{"1d20",1,20}, {"2d6",2,6}, {"3D8",3,8}, {"20d1000",20,1000},
	} {
		n, sides, err := parseDiceNotation(tc.input)
		if err != nil || n != tc.dice || sides != tc.sides {
			t.Errorf("parseDiceNotation(%q) = %d,%d,%v", tc.input,n,sides,err)
		}
	}
	for _, input := range []string{
		"", "2", "d6", "2d", "2d6d8", "0d6", "2d0", "1d1",
		"21d6", "2d1001", "-1d6", "2d-6", "1.5d6", "2d 6",
		"999999999999999999999999d6", "2d999999999999999999999",
	} {
		if _, _, err := parseDiceNotation(input); err == nil {
			t.Errorf("accepted malformed dice notation %q", input)
		}
	}
}

func TestDaemonRollResultContainsValidDiceAndTotal(t *testing.T) {
	user := User{Name:"Postal",Color:"#FFAA22"}
	result,err := newRollResult("room", user, "3d8")
	if err != nil || result.Type!="roll_result" || result.User!=user || result.ID=="" || result.Time.IsZero() {
		t.Fatalf("invalid daemon roll result: %+v, %v",result,err)
	}
	parts := strings.SplitN(result.Text, ": ",2)
	if len(parts)!=2 || parts[0]!="3d8" {
		t.Fatalf("dice label invalid: %q",result.Text)
	}
	chunks := strings.SplitN(parts[1]," = ",2)
	if len(chunks)!=2 { t.Fatalf("dice total missing: %q",result.Text) }
	rolls:=strings.Split(chunks[0]," + ")
	if len(rolls)!=3 { t.Fatalf("expected 3 individual rolls, got %q",result.Text) }
	total:=0
	for _,roll:=range rolls {
		n,err:=strconv.Atoi(roll)
		if err!=nil || n<1 || n>8 { t.Fatalf("invalid die %q in %q",roll,result.Text) }
		total+=n
	}
	if chunks[1]!=strconv.Itoa(total) { t.Fatalf("wrong total in %q",result.Text) }
	if _,err:=newRollResult("room",user,"200d10000"); err==nil {
		t.Fatal("daemon accepted excessive dice request")
	}
}

func TestFortuneResultPresentationAndPublicDocumentation(t *testing.T) {
	m:=newMessageInteractionTestModel(100)
	user:=User{Name:"Postal",Color:"#FFFFFF"}
	for _, target:=range []string{"","Postal"} {
		p,err:=newFortuneResult("lobby",user,target)
		if err!=nil || p.Type!="fortune_result" || p.ID=="" || p.FortuneTo!=target {
			t.Fatalf("invalid fortune result: %+v, %v",p,err)
		}
		known:=false
		for _,say:=range fortuneSayings { if p.Text==say {known=true;break} }
		if !known { t.Fatalf("unknown fortune saying: %q",p.Text) }
		rendered:=stripANSISequences(strings.Join(m.renderPacketBaseLines(p,100),"\n"))
		prefix:="Postal's Fortune: "
		if target!="" {prefix="Postal, your fortune is: "}
		if !strings.Contains(rendered,prefix) || !strings.Contains(rendered,p.Text) {
			t.Fatalf("fortune rendering incorrect: %q",rendered)
		}
		if !strings.Contains(rendered,"["+p.Time.Local().Format("15:04")+"]") {
			t.Fatalf("fortune timestamp missing: %q",rendered)
		}
		if target != "" {
			if len(p.Mentions)!=1 || p.Mentions[0]!=target {
				t.Fatalf("targeted fortune did not carry canonical mention: %+v",p.Mentions)
			}
			recipient := m
			recipient.user.Name = target
			if recipient.messageBackground(p)!=lipgloss.Color(recipient.cfg.Theme.MentionBackgroundColor) {
				t.Fatal("targeted fortune did not use mention background on recipient's screen")
			}
			bystander := m
			bystander.user.Name = "Bystander"
			if bystander.messageBackground(p)!=lipgloss.Color(bystander.cfg.Theme.Background) {
				t.Fatal("targeted fortune highlighted a bystander's screen")
			}
		} else if len(p.Mentions)!=0 {
			t.Fatalf("untargeted fortune should not carry mentions: %+v",p.Mentions)
		}
	}
	roll,err:=newRollResult("lobby",user,"1d6")
	if err!=nil {t.Fatal(err)}
	if plain:=stripANSISequences(strings.Join(m.renderPacketBaseLines(roll,100),"\n")); !strings.Contains(plain,"[ROLL] Postal rolled 1d6: ") {
		t.Fatalf("roll result not styled as a chat event: %q",plain)
	}
	for _,page:=range handbookPages {
		for _,entry:=range page.Entries {
			if strings.Contains(strings.ToLower(entry.Command+" "+entry.Description),"/fortune") {
				t.Fatal("undocumented fortune leaked into handbook")
			}
		}
	}
	foundRoll:=false
	for _,entry:=range handbookPages[0].Entries {if strings.HasPrefix(entry.Command,"/roll") {foundRoll=true}}
	if !foundRoll {t.Fatal("roll is missing from the CHAT handbook")}
}

func TestDaemonRollAndFortuneRequestsCannotForgeResults(t *testing.T) {
	s:=newServer()
	conn,peer:=net.Pipe()
	defer peer.Close()
	done:=make(chan struct{})
	go func(){s.handle(conn);close(done)}()
	encoder:=json.NewEncoder(peer)
	decoder:=json.NewDecoder(bufio.NewReader(peer))
	if err:=peer.SetReadDeadline(time.Now().Add(3*time.Second));err!=nil {t.Fatal(err)}
	if err:=encoder.Encode(Packet{Type:"hello",Room:"test",User:User{Name:"Alice",Color:"#FFFFFF"}});err!=nil {t.Fatal(err)}
	receiveUntil:=func(kind string) Packet {
		t.Helper()
		for {
			var packet Packet
			if err:=decoder.Decode(&packet);err!=nil {t.Fatalf("waiting for %s: %v",kind,err)}
			if packet.Type==kind {return packet}
		}
	}
	receiveUntil("users")
	if err:=encoder.Encode(Packet{
		Type:"roll",Room:"test",Text:"2d6",User:User{Name:"Impostor"},
	});err!=nil {t.Fatal(err)}
	roll:=receiveUntil("roll_result")
	if roll.User.Name!="Alice" || !regexp.MustCompile(`^2d6: [1-6] \+ [1-6] = ([2-9]|1[0-2])$`).MatchString(roll.Text) {
		t.Fatalf("roll allowed spoofed sender or result: %+v",roll)
	}
	receiveUntil("users")
	if err:=encoder.Encode(Packet{Type:"fortune",Room:"test",Text:"@alice",FortuneTo:"Impostor",User:User{Name:"Impostor"}});err!=nil {t.Fatal(err)}
	fortune:=receiveUntil("fortune_result")
	if fortune.User.Name!="Alice" || fortune.FortuneTo!="Alice" {
		t.Fatalf("fortune did not use canonical target/identity: %+v",fortune)
	}
	receiveUntil("users")
	if err:=encoder.Encode(Packet{Type:"fortune",Room:"test",Text:"@not-online"});err!=nil {t.Fatal(err)}
	response:=receiveUntil("system")
	if !strings.Contains(response.Text,"user must be online") {t.Fatalf("invalid target was not rejected: %+v",response)}
	for _,typeName:=range []string{"roll_result","fortune_result"} {
		if err:=encoder.Encode(Packet{Type:typeName,Text:"Forged!",User:User{Name:"Impostor"}});err!=nil {t.Fatal(err)}
	}
	if err:=peer.SetReadDeadline(time.Now().Add(100*time.Millisecond));err!=nil {t.Fatal(err)}
	var forged Packet
	if err:=decoder.Decode(&forged);err==nil {
		t.Fatalf("daemon broadcast client-forged result: %+v",forged)
	} else if ne,ok:=err.(net.Error);!ok || !ne.Timeout() {
		t.Fatalf("expected silence after forged result, got %v",err)
	}
	_ = peer.Close()
	select {case <-done:case <-time.After(time.Second):t.Fatal("daemon did not stop")}
}

func TestRollColorsUsernameAndDiceButNotVerbOrColon(t *testing.T) {
	m := newMessageInteractionTestModel(120)
	user := User{Name: "Postal", Color: "#FF3399"}
	packet := Packet{
		Type: "roll_result", User: user,
		Text: "2d6: 4 + 6 = 10", Time: time.Now().UTC(),
	}
	rendered := strings.Join(m.renderPacketBaseLines(packet, m.width), "\n")
	bg := m.messageBackground(packet)
	userStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(user.Color)).Background(bg)
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.TextColor)).Background(bg)
	for _, segment := range []string{
		userStyle.Render("Postal"),
		normalStyle.Render(" rolled "),
		userStyle.Render("2d6"),
		normalStyle.Render(": 4 + 6 = 10"),
	} {
		if !strings.Contains(rendered, segment) {
			t.Errorf("dice result missing correctly styled segment %q in %q", segment, rendered)
		}
	}
	for _, segment := range []string{
		userStyle.Render("Postal rolled"),
		userStyle.Render("2d6:"),
	} {
		if strings.Contains(rendered, segment) {
			t.Errorf("dice result colors wrong text with user color: %q", segment)
		}
	}
	if got := stripANSISequences(rendered); !strings.Contains(got, "Postal rolled 2d6: 4 + 6 = 10") {
		t.Fatalf("dice text changed while recoloring: %q", got)
	}
}

func TestFortuneSayingsAreDistinctAndNonempty(t *testing.T) {
	if len(fortuneSayings) < 150 {
		t.Fatalf("fortune oracle unexpectedly shrank to %d sayings", len(fortuneSayings))
	}
	seen := make(map[string]bool, len(fortuneSayings))
	for i, fortune := range fortuneSayings {
		trimmed := strings.TrimSpace(fortune)
		if trimmed == "" {
			t.Fatalf("fortune %d is empty", i)
		}
		if len([]rune(trimmed)) > 280 {
			t.Fatalf("fortune %d is too long for a short chat prophecy", i)
		}
		key := strings.ToLower(trimmed)
		if seen[key] {
			t.Fatalf("fortune %d is duplicated: %q", i, trimmed)
		}
		seen[key] = true
	}
}

func TestFortuneCollectionIncludesBruh(t *testing.T) {
	for _, saying := range fortuneSayings {
		if saying == "Bruh" {
			return
		}
	}
	t.Fatal("the oracle has forgotten Bruh")
}

func TestSpecialMessageTimestampsAndLeftAlignment(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	user := User{Name:"Postal",Color:"#ff3399"}
	stamp := time.Date(2026,time.January,2,15,4,0,0,time.Local)
	for _, packet := range []Packet{
		{Type:"roll_result",User:user,Text:"2d6: 4 + 6 = 10",Time:stamp},
		{Type:"fortune_result",User:user,Text:"Bruh",Time:stamp},
		{Type:"whisper_result",User:user,WhisperTo:"Alice",Text:"private",Time:stamp},
	} {
		m.user.Name = "Postal"
		lines := m.renderPacketBaseLines(packet,100)
		if len(lines)==0 {t.Fatalf("%s rendered no rows",packet.Type)}
		rendered := lines[0]
		if plain := stripANSISequences(rendered);!strings.HasPrefix(plain,"[15:04] ") {
			t.Errorf("%s timestamp has extra left padding: %q",packet.Type,plain)
		}
		stampStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.cfg.Theme.MutedTextColor)).
			Background(m.messageBackground(packet))
		if !strings.Contains(rendered,stampStyle.Render("[15:04] ")) {
			t.Errorf("%s timestamp does not use normal muted timestamp style",packet.Type)
		}
	}
}
