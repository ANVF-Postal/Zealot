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
		prefix:="Fortune: "
		if target!="" {prefix="Fortune to @Postal: "}
		if !strings.Contains(rendered,prefix) || !strings.Contains(rendered,p.Text) {
			t.Fatalf("fortune rendering incorrect: %q",rendered)
		}
		if !strings.Contains(rendered,"["+p.Time.Local().Format("15:04")+"]") {
			t.Fatalf("fortune timestamp missing: %q",rendered)
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
		Type:"roll",Room:"test",Text:"2d6: 6 + 6 = 12",User:User{Name:"Impostor"},
	});err!=nil {t.Fatal(err)}
	roll:=receiveUntil("roll_result")
	if roll.User.Name!="Alice" || !regexp.MustCompile(`^2d6: [1-6] \+ [1-6] = ([2-9]|1[0-2])$`).MatchString(roll.Text) {
		t.Fatalf("roll allowed spoofed sender or result: %+v",roll)
	}
	receiveUntil("users")
	if err:=encoder.Encode(Packet{Type:"fortune",Room:"test",Text:"@alice",User:User{Name:"Impostor"}});err!=nil {t.Fatal(err)}
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
