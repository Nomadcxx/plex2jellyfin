package transfer

import (
	"slices"
	"testing"
)

func TestBuildArgsAddsNoTimesByDefault(t *testing.T) {
	r := &RsyncTransferer{}
	args := r.buildArgs(TransferOptions{})
	if !slices.Contains(args, "--no-times") {
		t.Fatalf("expected --no-times in default args, got %v", args)
	}
}

func TestBuildArgsOmitsNoTimesWhenPreserveTimes(t *testing.T) {
	r := &RsyncTransferer{}
	args := r.buildArgs(TransferOptions{PreserveTimes: true})
	if slices.Contains(args, "--no-times") {
		t.Fatalf("expected no --no-times when PreserveTimes=true, got %v", args)
	}
}
