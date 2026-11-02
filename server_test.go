package main

import (
	"bufio"
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestServerClientSendFlushesBufferedPacket(t *testing.T) {
	serverConn, peerConn := net.Pipe()
	defer serverConn.Close()
	defer peerConn.Close()

	writer := bufio.NewWriterSize(serverConn, 64*1024)
	client := &serverClient{
		enc:    json.NewEncoder(writer),
		writer: writer,
	}

	sent := make(chan struct{})
	go func() {
		client.send(Packet{Type: "message", Text: "buffered"})
		close(sent)
	}()

	if err := peerConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var packet Packet
	if err := json.NewDecoder(peerConn).Decode(&packet); err != nil {
		t.Fatalf("could not read flushed packet: %v", err)
	}
	if packet.Type != "message" || packet.Text != "buffered" {
		t.Fatalf("received packet = %#v, want message with buffered text", packet)
	}
	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatal("server send did not finish after the peer read the packet")
	}
}
