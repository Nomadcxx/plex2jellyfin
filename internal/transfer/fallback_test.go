package transfer

import (
	"errors"
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
	result, err := ft.Move("/src", "/dst", DefaultOptions())

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
	result, err := ft.Move("/src", "/dst", DefaultOptions())

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
