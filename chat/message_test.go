package chat

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestMesg(t *testing.T) {
	ts := rand.Int63()
	s := fmt.Sprintf("TEXT MESSAGE %v", rand.Int63())
	mesg := &Message{Text: s, TTL: rand.Uint32()}
	m := &message{}
	m.encode(mesg, ts)
	nm, err := m.open()
	if err != nil {
		t.Fatal(err)
	}
	if nm.Timestamp != ts {
		t.Fatalf("timestamp filed does not have the same value")
	}
	if nm.TTL != mesg.TTL {
		t.Fatalf("ttl filed does not have the same value")
	}
	if nm.Room != mesg.Room {
		t.Fatal("room filed does not have the same value")
	}
	if nm.Nickname != mesg.Nickname {
		t.Fatal("nikname filed does not have the same value")
	}
	if nm.Text != mesg.Text {
		t.Fatal("text filed does not have the same value")
	}
}
