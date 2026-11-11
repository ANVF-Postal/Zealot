package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestPollDefinitionValidation(t *testing.T) {
	q, opts, err := parsePollDefinition(" Best language? | Go | Rust | Python ")
	if err != nil || q != "Best language?" || len(opts) != 3 || opts[1] != "Rust" {
		t.Fatalf("valid poll parse: %q %v %v", q, opts, err)
	}
	for _, input := range []string{
		"", "Question", "Question | one", " | Go | Rust",
		"Q | a | A", "Q | | Rust", "Q | Go | " + strings.Repeat("x",61),
		"Q | a | b | c | d | e | f | g", strings.Repeat("Q",161)+" | a | b",
	} {
		if _,_,err := parsePollDefinition(input);err==nil {
			t.Errorf("accepted invalid poll definition %q",input)
		}
	}
}

func newPollTestClient(name,id string, buffer *bytes.Buffer) *serverClient {
	return &serverClient{
		id:id, room:"polls", user:User{Name:name,Color:"#11CC88"},
		enc:json.NewEncoder(buffer),
	}
}

func takePollPackets(t *testing.T, buffer *bytes.Buffer) []Packet {
	t.Helper()
	dec:=json.NewDecoder(bytes.NewReader(buffer.Bytes()))
	var packets []Packet
	for dec.More() {
		var p Packet
		if err:=dec.Decode(&p);err!=nil {t.Fatal(err)}
		packets=append(packets,p)
	}
	buffer.Reset()
	return packets
}

func TestPollVotingIsAuthoritativeAndResultsRequireVoting(t *testing.T) {
	s:=newServer()
	room:=s.room("polls",true)
	var aliceOutput,bobOutput,carolOutput bytes.Buffer
	alice:=newPollTestClient("Alice","a",&aliceOutput)
	bob:=newPollTestClient("Bob","b",&bobOutput)
	carol:=newPollTestClient("Carol","c",&carolOutput)
	room.clients[alice.id]=alice
	room.clients[bob.id]=bob
	room.clients[carol.id]=carol

	s.createPoll("polls",alice,"Which? | Go | Rust")
	created:=takePollPackets(t,&aliceOutput)
	if len(created)!=1 || created[0].Type!="poll_result" || created[0].Poll==nil {
		t.Fatalf("poll create did not deliver: %+v",created)
	}
	p:=created[0].Poll
	if p.ID!="1" || p.Question!="Which?" || p.Choice!=0 || p.ResultsVisible || p.Total!=0 || len(p.Counts)!=0 {
		t.Fatalf("poll leaked results before voting: %+v",p)
	}
	takePollPackets(t,&bobOutput)
	takePollPackets(t,&carolOutput)
	s.votePoll("polls",bob,&PollData{ID:"1",Choice:2,Total:999,Counts:[]int{999,0},Closed:true,ResultsVisible:true})
	afterAlice:=takePollPackets(t,&aliceOutput)
	afterBob:=takePollPackets(t,&bobOutput)
	afterCarol:=takePollPackets(t,&carolOutput)
	if len(afterBob)!=1 || afterBob[0].Poll.Choice!=2 ||
		!afterBob[0].Poll.ResultsVisible || afterBob[0].Poll.Counts[1]!=1 ||
		afterBob[0].Poll.Total!=1 || afterBob[0].Poll.Closed {
		t.Fatalf("daemon accepted forged counts/status or lost vote: %+v",afterBob)
	}
	for _,packets:=range [][]Packet{afterAlice,afterCarol} {
		if len(packets)!=1 || packets[0].Poll.ResultsVisible || len(packets[0].Poll.Counts)!=0 {
			t.Fatalf("other voter received results before voting: %+v",packets)
		}
	}
	s.votePoll("polls",bob,&PollData{ID:"1",Choice:1})
	bobUpdate:=takePollPackets(t,&bobOutput)
	takePollPackets(t,&aliceOutput)
	takePollPackets(t,&carolOutput)
	if len(bobUpdate)!=1 || bobUpdate[0].Poll.Counts[0]!=1 || bobUpdate[0].Poll.Counts[1]!=0 ||
		bobUpdate[0].Poll.Total!=1 || bobUpdate[0].Poll.Choice!=1 {
		t.Fatalf("changed vote counted twice: %+v",bobUpdate)
	}
	s.votePoll("polls",carol,&PollData{ID:"1",Choice:2})
	takePollPackets(t,&bobOutput)
	takePollPackets(t,&aliceOutput)
	carolUpdate:=takePollPackets(t,&carolOutput)
	if len(carolUpdate)!=1 || carolUpdate[0].Poll.Total!=2 {
		t.Fatalf("second voter missing: %+v",carolUpdate)
	}
	if len(room.messages)!=1 || room.messages[0].Type!="poll_result" {
		t.Fatalf("room history should contain one poll, got %+v",room.messages)
	}
}

func TestOnlyCreatorMayClosePollAndClosedPollRejectsVotes(t *testing.T) {
	s:=newServer()
	room:=s.room("polls",true)
	var aOut,bOut bytes.Buffer
	a:=newPollTestClient("Alice","a",&aOut)
	b:=newPollTestClient("Bob","b",&bOut)
	room.clients[a.id]=a
	room.clients[b.id]=b
	s.createPoll("polls",a,"Go? | yes | no")
	takePollPackets(t,&aOut)
	takePollPackets(t,&bOut)
	s.closePoll("polls",b,"1")
	denied:=takePollPackets(t,&bOut)
	if len(denied)!=1 || denied[0].Type!="system" || room.polls["1"].closed {
		t.Fatalf("non-creator closed poll: %+v",denied)
	}
	s.closePoll("polls",a,"1")
	closed:=takePollPackets(t,&aOut)
	takePollPackets(t,&bOut)
	if len(closed)!=1 || !closed[0].Poll.Closed || !closed[0].Poll.ResultsVisible {
		t.Fatalf("close did not publish final state: %+v",closed)
	}
	s.votePoll("polls",b,&PollData{ID:"1",Choice:1})
	rejected:=takePollPackets(t,&bOut)
	if len(rejected)!=1 || rejected[0].Type!="system" {
		t.Fatalf("closed poll accepted vote: %+v",rejected)
	}
}

func TestPollHistoryPersonalizesVotesAndShowsActiveWithoutHistory(t *testing.T) {
	s:=newServer()
	room:=s.room("polls",true)
	var aOut bytes.Buffer
	a:=newPollTestClient("Alice","a",&aOut)
	room.clients[a.id]=a
	s.createPoll("polls",a,"Vote? | one | two")
	takePollPackets(t,&aOut)
	s.votePoll("polls",a,&PollData{ID:"1",Choice:2})
	takePollPackets(t,&aOut)
	unvoted:=s.personalizePollHistory(room,"Newcomer","polls")
	voted:=s.personalizePollHistory(room,"Alice","polls")
	if len(unvoted)!=1 || unvoted[0].Poll.ResultsVisible ||
		len(voted)!=1 || voted[0].Poll.Choice!=2 || voted[0].Poll.Counts[1]!=1 {
		t.Fatalf("history did not personalize vote: guest=%+v voter=%+v",unvoted,voted)
	}
	room.historyEnabled=false
	room.messages=nil
	open:=s.personalizePollHistory(room,"Newcomer","polls")
	if len(open)!=1 || open[0].Poll.ID!="1" {
		t.Fatalf("new arrival did not get active poll without history: %+v",open)
	}
}

func testPollPacket() Packet {
	return Packet{ID:"poll-message-1",Type:"poll_result",Time:time.Date(2026,time.January,2,15,4,0,0,time.UTC),
		User:User{Name:"Postal",Color:"#33BB99"},Poll:&PollData{ID:"7",Question:"Best language?",Options:[]string{"Go","Rust","Python"}}}
}

func TestPollLayoutClickAndInPlaceResults(t *testing.T) {
	for _,width:=range []int{40,90} {
		p:=testPollPacket()
		m:=newMessageInteractionTestModel(width,p)
		m.height=30
		m.rebuildMessageCache()
		rows,choices:=m.pollLayout(p,width)
		if len(rows)!=len(choices) || len(rows)!=m.renderPacketLineCount(p,width) {
			t.Fatalf("width %d: poll row geometry differs",width)
		}
		lineCount:=len(m.renderPacketBaseLines(p,width))
		if lineCount!=len(rows) {t.Fatalf("width %d: rendered rows=%d, layout=%d",width,lineCount,len(rows))}
		optionRow:=-1
		for i,n:=range choices {if n==2 {optionRow=i;break}}
		if optionRow<0 {t.Fatalf("width %d: no Rust hitbox",width)}
		left:=1
		if narrowBordersHidden(width) {left=0}
		if id,n:=m.pollOptionAt(left+3,2+optionRow);id!="7" || n!=2 {
			t.Fatalf("width %d: poll click=(%s,%d), want 7,2",width,id,n)
		}
		m.mousePositionKnown=true
		m.lastMouseX,m.lastMouseY=left+3,2+optionRow
		m.refreshMessageHover()
		if m.pollHoveredID!=p.ID || m.pollHoveredOption!=2 {
			t.Fatalf("width %d: hover did not select Rust",width)
		}
		updated:=p
		updated.Poll=&PollData{ID:"7",Question:"Best language?",Options:p.Poll.Options,Counts:[]int{0,2,1},Total:3,Choice:2,ResultsVisible:true}
		m.applyPollSnapshot(updated)
		if len(m.messages)!=1 || m.messages[0].Poll.Total!=3 {
			t.Fatalf("width %d: updated poll generated duplicate chat line",width)
		}
		rendered:=strings.Join(m.renderPollLines(updated,width),"\n")
		if !strings.Contains(stripANSISequences(rendered),"67%") {
			t.Fatalf("width %d: live results not rendered: %q",width,rendered)
		}
		if width>48 && !strings.Contains(stripANSISequences(rendered),"█") {
			t.Fatalf("width %d: ASCII bar missing",width)
		}
		for _,line:=range m.renderPollLines(updated,width) {
			if lipgloss.Width(line)>chatContentWidth(width) {
				t.Fatalf("width %d: bar overflows chat",width)
			}
		}
	}
}

func TestPollMouseVotingUsesExistingClickDispatcher(t *testing.T) {
	p:=testPollPacket()
	m:=newMessageInteractionTestModel(90,p)
	m.connected=true
	m.room="polls"
	var output bytes.Buffer
	m.client=&wireClient{enc:json.NewEncoder(&output)}
	_,choices:=m.pollLayout(p,m.width)
	row:=-1
	for i,n:=range choices {if n==1 {row=i;break}}
	if row<0 {t.Fatal("poll option not found")}
	updated,_:=m.Update(tea.MouseClickMsg{Button:tea.MouseLeft,X:4,Y:2+row})
	m=updated.(model)
	packets:=takePollPackets(t,&output)
	if len(packets)!=1 || packets[0].Type!="poll_vote" || packets[0].Poll.ID!="7" || packets[0].Poll.Choice!=1 {
		t.Fatalf("poll click not sent to daemon: %+v",packets)
	}
}
