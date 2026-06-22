package main

import "testing"

func TestRootRegistersHiddenPostmortemCommand(t *testing.T) {
	cmd := newRootCmd()
	names := subcommandNames(cmd)
	hidden := hiddenSubcommandMap(cmd)

	if !hasSubcommand("postmortem", names) {
		t.Fatalf("postmortem command not registered")
	}
	if !hidden["postmortem"] {
		t.Fatalf("postmortem should be hidden from normal help")
	}
}
