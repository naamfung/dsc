// Package hooks provides diagnostic generation passes for the type checker.
// Passes run after fixpoint iteration converges and inspect the fully-analyzed
// function results to generate type errors, warnings, and suggestions.
//
// # PASS ARCHITECTURE
//
// Each pass is a function that receives:
//   - Session: The analysis session with source name and settings
//   - FunctionExpr: The function AST node being checked
//   - FuncResult: Complete analysis results (types, flow facts, scopes)
//
// Passes return a slice of Diagnostics which are accumulated by the checker.
// Passes should not modify session state.
//
// AVAILABLE PASSES
//
//   - WithAssign: Type mismatch in assignments (local x: string = 123)
//   - WithReturn: Return type violations (function returning wrong type)
//   - WithCall: Argument type mismatches in function calls
//   - WithField: Invalid field access on types without the field
//   - WithControl: Unreachable code and control flow issues
//   - WithIdent: References to undefined identifiers
//
// # USAGE
//
// Register passes when creating the checker:
//
//	checker := check.NewChecker(db, deps, hooks.All()...)
//
// Or selectively:
//
//	checker := check.NewChecker(db, deps,
//	    hooks.WithAssign(),
//	    hooks.WithField(),
//	)
package hooks

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/diag"
)

// All returns all standard analysis passes.
func All() []check.Option {
	return []check.Option{
		WithAssign(),
		WithReturn(),
		WithCall(),
		WithField(),
		WithControl(),
		WithIdent(),
	}
}

// WithAssign enables assignment type checking.
func WithAssign() check.Option {
	return check.WithPass(func(sess *check.Session, _ *ast.FunctionExpr, result *api.FuncResult) []diag.Diagnostic {
		if result.NarrowSynth == nil || result.Graph == nil {
			return nil
		}
		return CheckAssignments(result.Graph, result.Scopes, result.NarrowSynth, result, sess.SourceName)
	})
}

// WithReturn enables return type checking.
func WithReturn() check.Option {
	return check.WithPass(func(sess *check.Session, fn *ast.FunctionExpr, result *api.FuncResult) []diag.Diagnostic {
		if result.NarrowSynth == nil {
			return nil
		}
		narrowView := result.NarrowSynth.Narrow()
		return CheckReturns(fn, result.Graph, result.Scopes, result.BaseScope, result.NarrowSynth, narrowView, sess.SourceName)
	})
}

// WithCall enables function call argument type checking.
func WithCall() check.Option {
	return check.WithPass(func(sess *check.Session, _ *ast.FunctionExpr, result *api.FuncResult) []diag.Diagnostic {
		if result.NarrowSynth == nil {
			return nil
		}
		narrowView := result.NarrowSynth.Narrow()
		return CheckCalls(result.Graph, result.Scopes, result.NarrowSynth, narrowView, sess.SourceName)
	})
}

// WithField enables field access checking.
func WithField() check.Option {
	return check.WithPass(func(sess *check.Session, _ *ast.FunctionExpr, result *api.FuncResult) []diag.Diagnostic {
		if result.NarrowSynth == nil || result.Graph == nil {
			return nil
		}
		narrowView := result.NarrowSynth.Narrow()
		return CheckFields(result.Graph, result.NarrowSynth, narrowView, sess.SourceName)
	})
}

// WithControl enables control flow validation.
func WithControl() check.Option {
	return check.WithPass(func(sess *check.Session, fn *ast.FunctionExpr, _ *api.FuncResult) []diag.Diagnostic {
		if fn == nil {
			return nil
		}
		return CheckControl(fn.Stmts, sess.SourceName)
	})
}

// WithIdent enables undefined identifier checking.
func WithIdent() check.Option {
	return check.WithPass(func(sess *check.Session, _ *ast.FunctionExpr, result *api.FuncResult) []diag.Diagnostic {
		if result.Graph == nil {
			return nil
		}
		return CheckIdents(result.Graph, result.Scopes, sess.SourceName)
	})
}
