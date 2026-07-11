package main

import (
	"reflect"
	"sort"
	"testing"

	"github.com/spf13/cobra"
)

func visibleSubcommandNames(cmdNames []string, cmdMap map[string]bool) []string {
	names := make([]string, 0, len(cmdNames))
	for _, name := range cmdNames {
		if !cmdMap[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func hiddenSubcommandMap(cmd *cobra.Command) map[string]bool {
	hidden := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		hidden[sub.Name()] = sub.Hidden
	}
	return hidden
}

func TestNewRootCmdShowsCoreManualCommandsOnly(t *testing.T) {
	cmd := newRootCmd()
	names := subcommandNames(cmd)
	visible := visibleSubcommandNames(names, hiddenSubcommandMap(cmd))

	want := []string{"config", "consolidate", "duplicates", "scan", "setup", "status", "trace", "version"}
	if !reflect.DeepEqual(visible, want) {
		t.Fatalf("visible root commands mismatch:\n got %v\nwant %v", visible, want)
	}
}

func TestNewRootCmdSuppressesUsageForRuntimeErrors(t *testing.T) {
	cmd := newRootCmd()
	if !cmd.SilenceUsage {
		t.Fatal("root command should suppress usage for runtime errors")
	}
}

func TestNewRootCmdKeepsAdvancedCommandsCallableButHidden(t *testing.T) {
	cmd := newRootCmd()
	names := subcommandNames(cmd)
	hidden := hiddenSubcommandMap(cmd)

	for _, required := range []string{
		"audit",
		"cleanup",
		"daemon",
		"database",
		"fix",
		"health",
		"migrate",
		"monitor",
		"orphans",
		"parses",
		"postmortem",
		"radarr",
		"repair",
		"review",
		"serve",
		"sonarr",
		"organize",
		"organize-folder",
		"validate",
		"watch",
	} {
		if !hasSubcommand(required, names) {
			t.Fatalf("expected root command to register %q, got %v", required, names)
		}
		if !hidden[required] {
			t.Fatalf("expected advanced command %q to be hidden from root help", required)
		}
	}

	count := 0
	for _, name := range names {
		if name == "orphans" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected orphans command registered once, got %d in %v", count, names)
	}
}
