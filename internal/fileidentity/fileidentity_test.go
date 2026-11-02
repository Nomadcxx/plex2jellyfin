package fileidentity

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFromInfoDistinguishesReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "media.mkv")
	if err := os.WriteFile(path, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, ok := FromPath(path)
	if !ok {
		t.Skip("platform does not expose device and inode fields")
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("other"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.Now().Add(time.Second), time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	second, ok := FromPath(path)
	if !ok {
		t.Fatal("identity disappeared for supported platform")
	}
	if first == second {
		t.Fatal("replacement file has the published file identity")
	}
}

func TestRemoveIfMatchesRestoresReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "media.mkv")
	if err := os.WriteFile(path, []byte("published"), 0o644); err != nil {
		t.Fatal(err)
	}
	identity, ok := FromLstat(path)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RemoveIfMatches(path, identity, true); err == nil {
		t.Fatal("RemoveIfMatches deleted a replacement file")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "replacement" {
		t.Fatalf("replacement = %q, error = %v", got, err)
	}
}

func TestRenameNoReplacePreservesDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mkv")
	destination := filepath.Join(root, "destination.mkv")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("destination"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := RenameNoReplace(source, destination)
	if errors.Is(err, ErrRenameNoReplaceUnsupported) {
		t.Skip(err)
	}
	if err == nil {
		t.Fatal("RenameNoReplace replaced an existing destination")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source after rejected rename: %v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil || string(got) != "destination" {
		t.Fatalf("destination = %q, error = %v", got, err)
	}
}

func TestExchangeSwapsFiles(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.mkv")
	second := filepath.Join(root, "second.mkv")
	if err := os.WriteFile(first, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}

	firstIdentity, ok := FromLstat(first)
	if !ok {
		t.Skip("platform does not expose stable file identities")
	}
	secondIdentity, ok := FromLstat(second)
	if !ok {
		t.Skip("platform does not expose stable file identities")
	}
	if err := Exchange(first, second); errors.Is(err, ErrExchangeUnsupported) {
		t.Skip(err)
	} else if err != nil {
		t.Fatal(err)
	}
	if !MatchesLstat(first, secondIdentity) || !MatchesLstat(second, firstIdentity) {
		t.Fatal("Exchange did not swap the path identities")
	}
	firstContents, err := os.ReadFile(first)
	if err != nil || string(firstContents) != "second" {
		t.Fatalf("first path = %q, error = %v", firstContents, err)
	}
	secondContents, err := os.ReadFile(second)
	if err != nil || string(secondContents) != "first" {
		t.Fatalf("second path = %q, error = %v", secondContents, err)
	}
}
