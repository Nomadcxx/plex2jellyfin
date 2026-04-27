package ipc

import (
	"path/filepath"
	"testing"
)

func TestOpLogAppendAndScan(t *testing.T) {
	dir := t.TempDir()
	lg, err := OpenOpLog(filepath.Join(dir, "op_log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := lg.Begin("op-1", "RESET_DB", map[string]any{"confirm": "media.db"}); err != nil {
		t.Fatal(err)
	}
	if err := lg.End("op-1", "done", ""); err != nil {
		t.Fatal(err)
	}
	if err := lg.Begin("op-2", "RESCAN", nil); err != nil {
		t.Fatal(err)
	}
	lg.Close()

	lg2, err := OpenOpLog(filepath.Join(dir, "op_log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer lg2.Close()
	pending, err := lg2.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != "op-2" {
		t.Errorf("pending = %+v", pending)
	}
}
