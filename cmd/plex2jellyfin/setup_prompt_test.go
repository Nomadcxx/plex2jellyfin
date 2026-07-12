package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestResolveModelChoice(t *testing.T) {
	opts := []string{"llama3", "qwen", "gemma4:31b"}
	cases := []struct {
		name    string
		answer  string
		def     string
		want    string
		wantErr bool
	}{
		{"empty uses default name", "", "qwen", "qwen", false},
		{"empty uses default index hint", "", "1", "llama3", false},
		{"pick by number", "2", "llama3", "qwen", false},
		{"pick by name", "gemma4:31b", "llama3", "gemma4:31b", false},
		{"out of range", "9", "llama3", "", true},
		{"unknown name still accepted", "custom:tag", "llama3", "custom:tag", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveModelChoice(tc.answer, opts, tc.def)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAskModelChoice_NumberedPick(t *testing.T) {
	var out bytes.Buffer
	p := newPrompter(strings.NewReader("2\n"), &out)
	defer p.Close()
	got, err := p.askModelChoice("Primary model", []string{"llama3", "qwen"}, "llama3", false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "qwen" {
		t.Fatalf("got %q, want qwen", got)
	}
	s := out.String()
	if !strings.Contains(s, "1) llama3") || !strings.Contains(s, "2) qwen") {
		t.Fatalf("expected numbered list:\n%s", s)
	}
}

func TestAskScanFrequency_BareMinutes(t *testing.T) {
	var out bytes.Buffer
	p := newPrompter(strings.NewReader("10\n"), &out)
	defer p.Close()
	got, err := p.askScanFrequency("5m")
	if err != nil {
		t.Fatal(err)
	}
	if got != "10m" {
		t.Fatalf("got %q, want 10m", got)
	}
}

func TestAskOctalMode_RejectsUsername(t *testing.T) {
	var out bytes.Buffer
	p := newPrompter(strings.NewReader("nomadx\n0644\n"), &out)
	defer p.Close()
	got, err := p.askOctalMode("File mode", "0644")
	if err != nil {
		t.Fatal(err)
	}
	if got != "0644" {
		t.Fatalf("got %q, want 0644", got)
	}
	if !strings.Contains(out.String(), "not an octal mode") {
		t.Fatalf("expected rejection message:\n%s", out.String())
	}
}

func TestNewPrompter_NonTTYUsesScanner(t *testing.T) {
	var out bytes.Buffer
	p := newPrompter(strings.NewReader("hello\n"), &out)
	defer p.Close()
	if p.useRL {
		t.Fatal("piped stdin must not enable readline")
	}
	got, err := p.ask("Label", "def")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Fatalf("got %q, want hello", got)
	}
}
