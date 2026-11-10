package transfer

import (
	"errors"
	"fmt"
	"os"
	"testing"
)

// mockTransferer is a test double for Transferer
type mockTransferer struct {
	name       string
	shouldFail bool
	canResume  bool
}

func (m *mockTransferer) Name() string    { return m.name }
func (m *mockTransferer) CanResume() bool { return m.canResume }

func (m *mockTransferer) Move(src, dst string, opts TransferOptions) (*TransferResult, error) {
	if m.shouldFail {
		return &TransferResult{Success: false}, errors.New("mock failure")
	}
	return &TransferResult{Success: true, BytesCopied: 100}, nil
}

func (m *mockTransferer) Copy(src, dst string, opts TransferOptions) (*TransferResult, error) {
	if m.shouldFail {
		return &TransferResult{Success: false}, errors.New("mock failure")
	}
	return &TransferResult{Success: true, BytesCopied: 100}, nil
}

func TestFallbackTransferer_FirstSucceeds(t *testing.T) {
	first := &mockTransferer{name: "first", shouldFail: false}
	second := &mockTransferer{name: "second", shouldFail: false}

	ft := NewFallbackTransferer(first, second)
	opts := DefaultOptions()
	opts.SkipHealthCheck = true
	result, err := ft.Move("/src", "/dst", opts)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if !result.Success {
		t.Fatal("Expected success")
	}
}

func TestFallbackTransferer_FallsBackOnFailure(t *testing.T) {
	first := &mockTransferer{name: "first", shouldFail: true}
	second := &mockTransferer{name: "second", shouldFail: false}

	ft := NewFallbackTransferer(first, second)
	opts := DefaultOptions()
	opts.SkipHealthCheck = true
	result, err := ft.Move("/src", "/dst", opts)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if !result.Success {
		t.Fatal("Expected success after fallback")
	}
}

func TestFallbackTransferer_AllFail(t *testing.T) {
	first := &mockTransferer{name: "first", shouldFail: true}
	second := &mockTransferer{name: "second", shouldFail: true}

	ft := NewFallbackTransferer(first, second)
	result, err := ft.Move("/src", "/dst", DefaultOptions())

	if err == nil {
		t.Fatal("Expected error when all backends fail")
	}
	if result.Success {
		t.Fatal("Expected failure")
	}
}

func TestFallbackTransferer_Name(t *testing.T) {
	first := &mockTransferer{name: "rsync"}
	second := &mockTransferer{name: "pv"}

	ft := NewFallbackTransferer(first, second)
	expected := "fallback(rsync,pv)"
	if ft.Name() != expected {
		t.Errorf("Expected name %q, got %q", expected, ft.Name())
	}
}

func TestFallbackTransferer_CanResume(t *testing.T) {
	// One can resume
	first := &mockTransferer{name: "rsync", canResume: true}
	second := &mockTransferer{name: "pv", canResume: false}

	ft := NewFallbackTransferer(first, second)
	if !ft.CanResume() {
		t.Error("Expected CanResume=true when at least one backend can resume")
	}

	// None can resume
	first.canResume = false
	ft2 := NewFallbackTransferer(first, second)
	if ft2.CanResume() {
		t.Error("Expected CanResume=false when no backend can resume")
	}
}

type eexistTransferer struct {
	name  string
	calls int
	err   error
}

func (m *eexistTransferer) Name() string    { return m.name }
func (m *eexistTransferer) CanResume() bool { return false }

func (m *eexistTransferer) Move(string, string, TransferOptions) (*TransferResult, error) {
	m.calls++
	if m.err != nil {
		return &TransferResult{Error: m.err, Attempts: 1}, m.err
	}
	return &TransferResult{Success: true, Attempts: 1}, nil
}

func (m *eexistTransferer) Copy(src, dst string, opts TransferOptions) (*TransferResult, error) {
	return m.Move(src, dst, opts)
}

func TestFallbackTransfererDoesNotRetryPublishEEXIST(t *testing.T) {
	publishErr := fmt.Errorf("commit temporary destination: %w", fmt.Errorf("publish without replacing destination: %w", os.ErrExist))
	first := &eexistTransferer{name: "rsync", err: publishErr}
	second := &eexistTransferer{name: "native"}
	ft := NewFallbackTransferer(first, second)
	opts := DefaultOptions()
	opts.SkipHealthCheck = true

	_, err := ft.Move("/src/movie.mkv", "/library/Movie.mkv", opts)
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("Move error = %v, want EEXIST", err)
	}
	if first.calls != 1 {
		t.Fatalf("first backend calls = %d, want 1", first.calls)
	}
	if second.calls != 0 {
		t.Fatalf("fallback retried EEXIST on %s (%d calls)", second.name, second.calls)
	}
}
