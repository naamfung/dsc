package hooks

import (
	"testing"
)

func TestAll_ReturnsOptions(t *testing.T) {
	opts := All()
	if len(opts) != 6 {
		t.Errorf("All() returned %d options, expected 6", len(opts))
	}
}

func TestWithAssign_NotNil(t *testing.T) {
	opt := WithAssign()
	if opt == nil {
		t.Error("WithAssign() returned nil")
	}
}

func TestWithReturn_NotNil(t *testing.T) {
	opt := WithReturn()
	if opt == nil {
		t.Error("WithReturn() returned nil")
	}
}

func TestWithCall_NotNil(t *testing.T) {
	opt := WithCall()
	if opt == nil {
		t.Error("WithCall() returned nil")
	}
}

func TestWithField_NotNil(t *testing.T) {
	opt := WithField()
	if opt == nil {
		t.Error("WithField() returned nil")
	}
}

func TestWithControl_NotNil(t *testing.T) {
	opt := WithControl()
	if opt == nil {
		t.Error("WithControl() returned nil")
	}
}

func TestWithIdent_NotNil(t *testing.T) {
	opt := WithIdent()
	if opt == nil {
		t.Error("WithIdent() returned nil")
	}
}

func TestNewLSPIndexer(t *testing.T) {
	indexer := NewLSPIndexer(nil, nil)
	if indexer == nil {
		t.Fatal("NewLSPIndexer returned nil")
	}
	if indexer.Symbols != nil {
		t.Error("Expected nil Symbols")
	}
	if indexer.CallGraph != nil {
		t.Error("Expected nil CallGraph")
	}
}

func TestWithLSPIndex_NilIndexer(t *testing.T) {
	opt := WithLSPIndex(nil)
	if opt == nil {
		t.Error("WithLSPIndex(nil) returned nil")
	}
}

func TestWithLSPIndex_NilSymbols(t *testing.T) {
	indexer := &LSPIndexer{Symbols: nil, CallGraph: nil}
	opt := WithLSPIndex(indexer)
	if opt == nil {
		t.Error("WithLSPIndex returned nil")
	}
}
