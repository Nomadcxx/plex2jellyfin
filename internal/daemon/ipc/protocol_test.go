package ipc

import (
	"encoding/json"
	"testing"
)

func TestRequestRoundtrip(t *testing.T) {
	r := Request{V: 1, ID: "abc", Cmd: CmdStatus}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var got Request
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.V != r.V || got.ID != r.ID || got.Cmd != r.Cmd {
		t.Errorf("roundtrip mismatch: %+v != %+v", got, r)
	}
}

func TestErrorCodeIsString(t *testing.T) {
	if string(ErrBusy) != "BUSY" {
		t.Errorf("ErrBusy = %q, want BUSY", ErrBusy)
	}
}

func TestLifecycleCommandsDefined(t *testing.T) {
	for _, c := range []Command{CmdStop, CmdRescan, CmdResetDB, CmdAttach, CmdCancel, CmdRecover} {
		if string(c) == "" {
			t.Errorf("command constant empty")
		}
	}
}
