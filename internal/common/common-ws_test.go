package common

import (
	"testing"

	"github.com/fxamacker/cbor/v2"
)

func TestSSHStreamMessageRoundTrip(t *testing.T) {
	want := SSHStreamMessage{Magic: SSHStreamMagic, StreamID: 1, Type: SSHStreamData, Data: []byte("ssh")}
	data, err := cbor.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got SSHStreamMessage
	if err := cbor.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Magic != want.Magic || got.StreamID != want.StreamID || got.Type != want.Type || string(got.Data) != string(want.Data) {
		t.Fatalf("round trip mismatch: %#v", got)
	}
}
