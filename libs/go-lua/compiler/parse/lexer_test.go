package parse

import (
	"errors"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

const (
	testTypeNumber = "number"
	testTypeString = "string"
)

func TestScanArrow(t *testing.T) {
	lexer := &Lexer{NewScanner(strings.NewReader("->"), "test"), nil, false, ast.Token{}, TNil, nil}
	tok, err := lexer.scanner.Scan(lexer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Type != TArrow {
		t.Errorf("expected TArrow, got %d (%s)", tok.Type, tok.Name)
	}
	if tok.Str != "->" {
		t.Errorf("expected '->', got %q", tok.Str)
	}
}

func TestScanQuestion(t *testing.T) {
	lexer := &Lexer{NewScanner(strings.NewReader("?"), "test"), nil, false, ast.Token{}, TNil, nil}
	tok, err := lexer.scanner.Scan(lexer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Type != TQuestion {
		t.Errorf("expected TQuestion, got %d (%s)", tok.Type, tok.Name)
	}
	if tok.Str != "?" {
		t.Errorf("expected '?', got %q", tok.Str)
	}
}

func TestScanBang(t *testing.T) {
	lexer := &Lexer{NewScanner(strings.NewReader("!"), "test"), nil, false, ast.Token{}, TNil, nil}
	tok, err := lexer.scanner.Scan(lexer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Type != TBang {
		t.Errorf("expected TBang, got %d (%s)", tok.Type, tok.Name)
	}
	if tok.Str != "!" {
		t.Errorf("expected '!', got %q", tok.Str)
	}
}

func TestScanInterface(t *testing.T) {
	lexer := &Lexer{NewScanner(strings.NewReader("interface"), "test"), nil, false, ast.Token{}, TNil, nil}
	tok, err := lexer.scanner.Scan(lexer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Type != TInterface {
		t.Errorf("expected TInterface, got %d (%s)", tok.Type, tok.Name)
	}
}

func TestScanReadonly(t *testing.T) {
	lexer := &Lexer{NewScanner(strings.NewReader("readonly"), "test"), nil, false, ast.Token{}, TNil, nil}
	tok, err := lexer.scanner.Scan(lexer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Type != TReadonly {
		t.Errorf("expected TReadonly, got %d (%s)", tok.Type, tok.Name)
	}
}

func TestScanTypeAsIdent(t *testing.T) {
	// "type" should be scanned as TIdent since it's a contextual keyword
	lexer := &Lexer{NewScanner(strings.NewReader("type"), "test"), nil, false, ast.Token{}, TNil, nil}
	tok, err := lexer.scanner.Scan(lexer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Type != TIdent {
		t.Errorf("expected TIdent for 'type', got %d (%s)", tok.Type, tok.Name)
	}
	if tok.Str != "type" {
		t.Errorf("expected 'type', got %q", tok.Str)
	}
}

func TestScanMinusNotArrow(t *testing.T) {
	// Just '-' should be scanned as minus, not arrow
	lexer := &Lexer{NewScanner(strings.NewReader("- 1"), "test"), nil, false, ast.Token{}, TNil, nil}
	tok, err := lexer.scanner.Scan(lexer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Type != '-' {
		t.Errorf("expected '-', got %d (%s)", tok.Type, tok.Name)
	}
}

func TestScanTypeAnnotationSequence(t *testing.T) {
	// Scan a type annotation: ": number?"
	input := ": " + testTypeNumber + "?"
	lexer := &Lexer{NewScanner(strings.NewReader(input), "test"), nil, false, ast.Token{}, TNil, nil}

	// Colon
	tok, err := lexer.scanner.Scan(lexer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Type != ':' {
		t.Errorf("expected ':', got %d (%s)", tok.Type, tok.Name)
	}

	// number
	tok, err = lexer.scanner.Scan(lexer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Type != TIdent || tok.Str != testTypeNumber {
		t.Errorf("expected TIdent '%s', got %d %q", testTypeNumber, tok.Type, tok.Str)
	}

	// ?
	tok, err = lexer.scanner.Scan(lexer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Type != TQuestion {
		t.Errorf("expected TQuestion, got %d (%s)", tok.Type, tok.Name)
	}
}

func TestScanFunctionTypeSequence(t *testing.T) {
	// Scan: (number) -> string
	input := "(" + testTypeNumber + ") -> " + testTypeString
	lexer := &Lexer{NewScanner(strings.NewReader(input), "test"), nil, false, ast.Token{}, TNil, nil}

	expected := []struct {
		typ int
		str string
	}{
		{'(', "("},
		{TIdent, testTypeNumber},
		{')', ")"},
		{TArrow, "->"},
		{TIdent, testTypeString},
	}

	for i, exp := range expected {
		tok, err := lexer.scanner.Scan(lexer)
		if err != nil {
			t.Fatalf("token %d: unexpected error: %v", i, err)
		}
		if tok.Type != exp.typ {
			t.Errorf("token %d: expected type %d, got %d", i, exp.typ, tok.Type)
		}
		if tok.Str != exp.str {
			t.Errorf("token %d: expected %q, got %q", i, exp.str, tok.Str)
		}
	}
}

func TestScanAs(t *testing.T) {
	lexer := &Lexer{NewScanner(strings.NewReader("as"), "test"), nil, false, ast.Token{}, TNil, nil}
	tok, err := lexer.scanner.Scan(lexer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Type != TAs {
		t.Errorf("expected TAs, got %d (%s)", tok.Type, tok.Name)
	}
}

func TestScanAllOperators(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		str      string
	}{
		{"==", TEqeq, "=="},
		{"~=", TNeq, "~="},
		{"<=", TLte, "<="},
		{">=", TGte, ">="},
		{"..", T2Comma, ".."},
		{"...", T3Comma, "..."},
		{"::", T2Colon, "::"},
		{"<<", TShl, "<<"},
		{">>", TShr, ">>"},
		{"//", TIdiv, "//"},
		{"->", TArrow, "->"},
		{"?", TQuestion, "?"},
		{"!", TBang, "!"},
		{"+", '+', "+"},
		{"-", '-', "-"},
		{"*", '*', "*"},
		{"/", '/', "/"},
		{"%", '%', "%"},
		{"^", '^', "^"},
		{"#", '#', "#"},
		{"&", '&', "&"},
		{"|", '|', "|"},
		{"~", '~', "~"},
		{"<", '<', "<"},
		{">", '>', ">"},
		{"=", '=', "="},
		{"(", '(', "("},
		{")", ')', ")"},
		{"{", '{', "{"},
		{"}", '}', "}"},
		{"[", '[', "["},
		{"]", ']', "]"},
		{";", ';', ";"},
		{",", ',', ","},
		{":", ':', ":"},
	}

	for _, tt := range tests {
		lexer := &Lexer{NewScanner(strings.NewReader(tt.input), "test"), nil, false, ast.Token{}, TNil, nil}
		tok, err := lexer.scanner.Scan(lexer)
		if err != nil {
			t.Errorf("Scan(%q) error: %v", tt.input, err)
			continue
		}
		if tok.Type != tt.expected {
			t.Errorf("Scan(%q) type = %d, want %d", tt.input, tok.Type, tt.expected)
		}
		if tok.Str != tt.str {
			t.Errorf("Scan(%q) str = %q, want %q", tt.input, tok.Str, tt.str)
		}
	}
}

func TestScanKeywords(t *testing.T) {
	keywords := map[string]int{
		"and":       TAnd,
		"break":     TBreak,
		"do":        TDo,
		"else":      TElse,
		"elseif":    TElseIf,
		"end":       TEnd,
		"false":     TFalse,
		"for":       TFor,
		"function":  TFunction,
		"if":        TIf,
		"in":        TIn,
		"local":     TLocal,
		"nil":       TNil,
		"not":       TNot,
		"or":        TOr,
		"return":    TReturn,
		"repeat":    TRepeat,
		"then":      TThen,
		"true":      TTrue,
		"until":     TUntil,
		"while":     TWhile,
		"goto":      TGoto,
		"interface": TInterface,
		"readonly":  TReadonly,
		"as":        TAs,
	}

	for kw, expected := range keywords {
		lexer := &Lexer{NewScanner(strings.NewReader(kw), "test"), nil, false, ast.Token{}, TNil, nil}
		tok, err := lexer.scanner.Scan(lexer)
		if err != nil {
			t.Errorf("Scan(%q) error: %v", kw, err)
			continue
		}
		if tok.Type != expected {
			t.Errorf("Scan(%q) type = %d (%s), want %d", kw, tok.Type, tok.Name, expected)
		}
	}
}

func TestScanNumbers(t *testing.T) {
	tests := []string{
		"0", "1", "123", "0.5", "3.14159", "1e10", "1E10",
		"1.5e-3", "0x1F", "0xABCDEF", "0X1a2b",
	}
	for _, input := range tests {
		lexer := &Lexer{NewScanner(strings.NewReader(input), "test"), nil, false, ast.Token{}, TNil, nil}
		tok, err := lexer.scanner.Scan(lexer)
		if err != nil {
			t.Errorf("Scan(%q) error: %v", input, err)
			continue
		}
		if tok.Type != TNumber {
			t.Errorf("Scan(%q) type = %d, want TNumber", input, tok.Type)
		}
	}
}

func TestScanStrings(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"hello"`, "hello"},
		{`'world'`, "world"},
		{`"with\nnewline"`, "with\nnewline"},
		{`"with\ttab"`, "with\ttab"},
		{`"escaped\""`, `escaped"`},
		{`[[multiline]]`, "multiline"},
		{`[=[with]=equals]=]`, "with]=equals"},
	}
	for _, tt := range tests {
		lexer := &Lexer{NewScanner(strings.NewReader(tt.input), "test"), nil, false, ast.Token{}, TNil, nil}
		tok, err := lexer.scanner.Scan(lexer)
		if err != nil {
			t.Errorf("Scan(%q) error: %v", tt.input, err)
			continue
		}
		if tok.Type != TString {
			t.Errorf("Scan(%q) type = %d, want TString", tt.input, tok.Type)
		}
		if tok.Str != tt.expected {
			t.Errorf("Scan(%q) str = %q, want %q", tt.input, tok.Str, tt.expected)
		}
	}
}

func TestScanComments(t *testing.T) {
	// Comments should be skipped, returning next token
	tests := []struct {
		input    string
		expected int
	}{
		{"-- comment\n42", TNumber},
		{"--[[multiline comment]]42", TNumber},
		{"--[=[nested]=]42", TNumber},
	}
	for _, tt := range tests {
		lexer := &Lexer{NewScanner(strings.NewReader(tt.input), "test"), nil, false, ast.Token{}, TNil, nil}
		tok, err := lexer.scanner.Scan(lexer)
		if err != nil {
			t.Errorf("Scan(%q) error: %v", tt.input, err)
			continue
		}
		if tok.Type != tt.expected {
			t.Errorf("Scan(%q) type = %d, want %d", tt.input, tok.Type, tt.expected)
		}
	}
}

func TestScanIdentifiers(t *testing.T) {
	tests := []string{
		"x", "foo", "bar123", "_private", "__double", "CamelCase",
		"with_underscore", "a1b2c3",
	}
	for _, input := range tests {
		lexer := &Lexer{NewScanner(strings.NewReader(input), "test"), nil, false, ast.Token{}, TNil, nil}
		tok, err := lexer.scanner.Scan(lexer)
		if err != nil {
			t.Errorf("Scan(%q) error: %v", input, err)
			continue
		}
		if tok.Type != TIdent {
			t.Errorf("Scan(%q) type = %d, want TIdent", input, tok.Type)
		}
		if tok.Str != input {
			t.Errorf("Scan(%q) str = %q", input, tok.Str)
		}
	}
}

func TestScanInvalidToken(t *testing.T) {
	lexer := &Lexer{NewScanner(strings.NewReader("$"), "test"), nil, false, ast.Token{}, TNil, nil}
	_, err := lexer.scanner.Scan(lexer)
	if err == nil {
		t.Error("expected error for invalid token '$'")
	}
}

func TestTokenName(t *testing.T) {
	// Test that TokenName returns something sensible
	names := []struct {
		tok  int
		name string
	}{
		{TAnd, "TAnd"},
		{TBreak, "TBreak"},
		{TArrow, "TArrow"},
		{TQuestion, "TQuestion"},
		{TBang, "TBang"},
		{'+', "+"},
		{'-', "-"},
	}
	for _, tt := range names {
		got := TokenName(tt.tok)
		if got == "" {
			t.Errorf("TokenName(%d) returned empty string", tt.tok)
		}
	}
}

func TestScanErrors(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{`"unterminated`, "unterminated string"},
		{`'unterminated`, "unterminated single-quoted string"},
		{"[[unterminated", "unterminated multiline string"},
		{"0x", "incomplete hex number"},
		{"$", "invalid character dollar"},
	}

	for _, tt := range tests {
		lexer := &Lexer{NewScanner(strings.NewReader(tt.input), "test"), nil, false, ast.Token{}, TNil, nil}
		_, err := lexer.scanner.Scan(lexer)
		if err == nil {
			t.Errorf("Scan(%q) [%s] expected error, got nil", tt.input, tt.desc)
		}
	}
}

func TestScanEscapeSequences(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"\a"`, "\a"},
		{`"\b"`, "\b"},
		{`"\f"`, "\f"},
		{`"\n"`, "\n"},
		{`"\r"`, "\r"},
		{`"\t"`, "\t"},
		{`"\v"`, "\v"},
		{`"\\"`, "\\"},
		{`"\'"`, "'"},
		{`"\""`, "\""},
		{`"\065"`, "A"},   // decimal escape
		{`"\x41"`, "x41"}, // Lua doesn't have \x, treated as literal
		{`"\z"`, "z"},     // unknown escape = literal
	}

	for _, tt := range tests {
		lexer := &Lexer{NewScanner(strings.NewReader(tt.input), "test"), nil, false, ast.Token{}, TNil, nil}
		tok, err := lexer.scanner.Scan(lexer)
		if err != nil {
			t.Errorf("Scan(%q) error: %v", tt.input, err)
			continue
		}
		if tok.Str != tt.expected {
			t.Errorf("Scan(%q) = %q, want %q", tt.input, tok.Str, tt.expected)
		}
	}
}

func TestScanWhitespaceAndNewlines(t *testing.T) {
	// Whitespace should be skipped
	tests := []struct {
		input    string
		expected int
	}{
		{"   42", TNumber},
		{"\t\t42", TNumber},
		{"\n\n42", TNumber},
		{"\r\n42", TNumber},
		{"  \t\n  42", TNumber},
	}

	for _, tt := range tests {
		lexer := &Lexer{NewScanner(strings.NewReader(tt.input), "test"), nil, false, ast.Token{}, TNil, nil}
		tok, err := lexer.scanner.Scan(lexer)
		if err != nil {
			t.Errorf("Scan(%q) error: %v", tt.input, err)
			continue
		}
		if tok.Type != tt.expected {
			t.Errorf("Scan(%q) type = %d, want %d", tt.input, tok.Type, tt.expected)
		}
	}
}

func TestScanEOF(t *testing.T) {
	lexer := &Lexer{NewScanner(strings.NewReader(""), "test"), nil, false, ast.Token{}, TNil, nil}
	tok, err := lexer.scanner.Scan(lexer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Type != EOF {
		t.Errorf("expected EOF, got %d", tok.Type)
	}
}

func TestScannerPosition(t *testing.T) {
	input := "local x\ny = 1"
	lexer := &Lexer{NewScanner(strings.NewReader(input), "test.lua"), nil, false, ast.Token{}, TNil, nil}

	// First token: local (line 1)
	tok, _ := lexer.scanner.Scan(lexer)
	if tok.Pos.Line != 1 {
		t.Errorf("'local' line = %d, want 1", tok.Pos.Line)
	}
	if tok.Pos.Source != "test.lua" {
		t.Errorf("source = %q, want 'test.lua'", tok.Pos.Source)
	}

	// Skip 'x'
	_, _ = lexer.scanner.Scan(lexer)

	// 'y' should be on line 2
	tok, _ = lexer.scanner.Scan(lexer)
	if tok.Pos.Line != 2 {
		t.Errorf("'y' line = %d, want 2", tok.Pos.Line)
	}
}

func TestLexerInterface(t *testing.T) {
	// Test the Lexer.Lex method used by yacc
	lexer := &Lexer{NewScanner(strings.NewReader("local x = 1"), "test"), nil, false, ast.Token{}, TNil, nil}

	var lval yySymType
	tok := lexer.Lex(&lval)
	if tok != TLocal {
		t.Errorf("first token = %d, want TLocal", tok)
	}
	if lval.token.Str != "local" {
		t.Errorf("token str = %q, want 'local'", lval.token.Str)
	}
}

func TestErrorFormat(t *testing.T) {
	// Test Error struct formatting
	err := &Error{
		Pos:     ast.Position{Source: "test.lua", Line: 10, Column: 5},
		Message: "test error",
		Token:   "foo",
	}
	str := err.Error()
	if str == "" {
		t.Error("Error.Error() returned empty string")
	}
	if !strings.Contains(str, "test.lua") {
		t.Error("error should contain source name")
	}
	if !strings.Contains(str, "10") {
		t.Error("error should contain line number")
	}

	// Test EOF error format
	errEOF := &Error{
		Pos:     ast.Position{Source: "test.lua", Line: EOF},
		Message: "unexpected EOF",
	}
	strEOF := errEOF.Error()
	if !strings.Contains(strEOF, "EOF") {
		t.Error("EOF error should mention EOF")
	}
}

func TestErrorRender(t *testing.T) {
	source := "local x = 1\nlocal y = @\nlocal z = 3"
	err := &Error{
		Pos:     ast.Position{Source: "test.lua", Line: 2, Column: 11},
		Message: "unexpected character '@'",
		Token:   "@",
		Source:  source,
	}
	rendered := err.Render()
	if rendered == "" {
		t.Error("Render() returned empty string")
	}
	if !strings.Contains(rendered, "error:") {
		t.Error("rendered should contain 'error:'")
	}
	if !strings.Contains(rendered, "test.lua") {
		t.Error("rendered should contain source file")
	}
	if !strings.Contains(rendered, "local y = @") {
		t.Error("rendered should contain source line")
	}
	if !strings.Contains(rendered, "^") {
		t.Error("rendered should contain caret pointer")
	}
}

func TestErrorRenderNoSource(t *testing.T) {
	err := &Error{
		Pos:     ast.Position{Source: "test.lua", Line: 10, Column: 5},
		Message: "test error",
		Token:   "foo",
		Source:  "",
	}
	rendered := err.Render()
	// Without source, should fall back to Error()
	if rendered != err.Error() {
		t.Error("Render() without source should fall back to Error()")
	}
}

func TestErrorRenderEdgeCases(t *testing.T) {
	// Test with column out of range
	source := "short"
	err := &Error{
		Pos:     ast.Position{Source: "test.lua", Line: 1, Column: 100},
		Message: "test",
		Token:   "",
		Source:  source,
	}
	rendered := err.Render()
	if !strings.Contains(rendered, "^") {
		t.Error("rendered should still contain caret")
	}

	// Test with column < 1
	err2 := &Error{
		Pos:     ast.Position{Source: "test.lua", Line: 1, Column: 0},
		Message: "test",
		Token:   "",
		Source:  source,
	}
	rendered2 := err2.Render()
	if !strings.Contains(rendered2, "^") {
		t.Error("rendered should contain caret for column 0")
	}

	// Test with multi-char token
	err3 := &Error{
		Pos:     ast.Position{Source: "test.lua", Line: 1, Column: 1},
		Message: "test",
		Token:   "local",
		Source:  source,
	}
	rendered3 := err3.Render()
	if !strings.Contains(rendered3, "~") {
		t.Error("rendered should contain tilde for multi-char token")
	}
}

func TestFriendlyTokenName(t *testing.T) {
	// Test known token names
	tests := []struct {
		tokIdx   int
		contains string
	}{
		{TAnd, ""}, // Just ensure it returns something
		{TBreak, ""},
		{TIdent, ""},
		{TNumber, ""},
		{TString, ""},
	}

	for _, tt := range tests {
		name := FriendlyTokenName(tt.tokIdx)
		if name == "" {
			t.Errorf("FriendlyTokenName(%d) returned empty string", tt.tokIdx)
		}
	}
}

func TestParseString(t *testing.T) {
	// Test successful parse
	source := "local x = 1"
	stmts, err := ParseString(source, "test.lua")
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}
	if len(stmts) != 1 {
		t.Errorf("expected 1 statement, got %d", len(stmts))
	}
}

func TestParseStringError(t *testing.T) {
	// Test parse error with source included
	source := "local x = @"
	_, err := ParseString(source, "test.lua")
	if err == nil {
		t.Fatal("expected error for invalid syntax")
	}
	var parseErr *Error
	if !errors.As(err, &parseErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if parseErr.Source != source {
		t.Error("parse error should include source")
	}
	rendered := parseErr.Render()
	if !strings.Contains(rendered, "local x = @") {
		t.Error("rendered error should show source line")
	}
}

func TestScanHexNumbers(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"0x1", true},
		{"0X1", true},
		{"0xABCDEF", true},
		{"0x1p10", true},
		{"0x1.5p10", true},
		{"0x1P10", true},
		{"0x1.5P-10", true},
	}
	for _, tt := range tests {
		lexer := &Lexer{NewScanner(strings.NewReader(tt.input), "test"), nil, false, ast.Token{}, TNil, nil}
		tok, err := lexer.scanner.Scan(lexer)
		if tt.valid {
			if err != nil {
				t.Errorf("Scan(%q) error: %v", tt.input, err)
				continue
			}
			if tok.Type != TNumber {
				t.Errorf("Scan(%q) type = %d, want TNumber", tt.input, tok.Type)
			}
		}
	}
}

func TestScanLongStrings(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"[[hello]]", "hello"},
		{"[=[hello]=]", "hello"},
		{"[==[hello]==]", "hello"},
		{"[[multi\nline]]", "multi\nline"},
		{"[[\nhello]]", "hello"},
		{"[[\nhello\n]]", "hello\n"},
	}
	for _, tt := range tests {
		lexer := &Lexer{NewScanner(strings.NewReader(tt.input), "test"), nil, false, ast.Token{}, TNil, nil}
		tok, err := lexer.scanner.Scan(lexer)
		if err != nil {
			t.Errorf("Scan(%q) error: %v", tt.input, err)
			continue
		}
		if tok.Type != TString {
			t.Errorf("Scan(%q) type = %d, want TString", tt.input, tok.Type)
		}
		if tok.Str != tt.expected {
			t.Errorf("Scan(%q) str = %q, want %q", tt.input, tok.Str, tt.expected)
		}
	}
}

func TestScanLongComments(t *testing.T) {
	tests := []string{
		"--[[ comment ]] 42",
		"--[=[ comment ]=] 42",
		"--[==[ comment ]==] 42",
		"--[[ multi\nline\ncomment ]] 42",
	}
	for _, input := range tests {
		lexer := &Lexer{NewScanner(strings.NewReader(input), "test"), nil, false, ast.Token{}, TNil, nil}
		tok, err := lexer.scanner.Scan(lexer)
		if err != nil {
			t.Errorf("Scan(%q) error: %v", input, err)
			continue
		}
		if tok.Type != TNumber {
			t.Errorf("Scan(%q) type = %d, want TNumber", input, tok.Type)
		}
	}
}

func TestScanDecimalEscapes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"\65"`, "A"},
		{`"\066"`, "B"},
		{`"\097"`, "a"},
		{`"\049\050\051"`, "123"},
	}
	for _, tt := range tests {
		lexer := &Lexer{NewScanner(strings.NewReader(tt.input), "test"), nil, false, ast.Token{}, TNil, nil}
		tok, err := lexer.scanner.Scan(lexer)
		if err != nil {
			t.Errorf("Scan(%q) error: %v", tt.input, err)
			continue
		}
		if tok.Str != tt.expected {
			t.Errorf("Scan(%q) str = %q, want %q", tt.input, tok.Str, tt.expected)
		}
	}
}

func TestScanHexStringEscapes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"\x41"`, "x41"},
		{`"\x4A"`, "x4A"},
	}
	for _, tt := range tests {
		lexer := &Lexer{NewScanner(strings.NewReader(tt.input), "test"), nil, false, ast.Token{}, TNil, nil}
		tok, err := lexer.scanner.Scan(lexer)
		if err != nil {
			t.Errorf("Scan(%q) error: %v", tt.input, err)
			continue
		}
		if tok.Str != tt.expected {
			t.Errorf("Scan(%q) str = %q, want %q", tt.input, tok.Str, tt.expected)
		}
	}
}

func TestScanFloatNumbers(t *testing.T) {
	tests := []string{
		"3.14", ".5", "1.", "1e10", "1E10", "1e+10", "1e-10",
		"1.5e10", ".5e10", "1.e10",
	}
	for _, input := range tests {
		lexer := &Lexer{NewScanner(strings.NewReader(input), "test"), nil, false, ast.Token{}, TNil, nil}
		tok, err := lexer.scanner.Scan(lexer)
		if err != nil {
			t.Errorf("Scan(%q) error: %v", input, err)
			continue
		}
		if tok.Type != TNumber {
			t.Errorf("Scan(%q) type = %d, want TNumber", input, tok.Type)
		}
	}
}

func TestScanUnterminatedStrings(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{`"unclosed`, "double quote"},
		{`'unclosed`, "single quote"},
		{`"with\nnewline`, "string with newline"},
	}
	for _, tt := range tests {
		lexer := &Lexer{NewScanner(strings.NewReader(tt.input), "test"), nil, false, ast.Token{}, TNil, nil}
		_, err := lexer.scanner.Scan(lexer)
		if err == nil {
			t.Errorf("Scan(%q) [%s] expected error", tt.input, tt.desc)
		}
	}
}

func TestScanUnterminatedLongStrings(t *testing.T) {
	tests := []string{
		"[[unclosed",
		"[=[unclosed",
		"[==[unclosed]=]",
	}
	for _, input := range tests {
		lexer := &Lexer{NewScanner(strings.NewReader(input), "test"), nil, false, ast.Token{}, TNil, nil}
		_, err := lexer.scanner.Scan(lexer)
		if err == nil {
			t.Errorf("Scan(%q) expected error for unclosed long string", input)
		}
	}
}

func TestScanLineComments(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"-- comment\n42", TNumber},
		{"--comment\n42", TNumber},
		{"-- comment at end", EOF},
	}
	for _, tt := range tests {
		lexer := &Lexer{NewScanner(strings.NewReader(tt.input), "test"), nil, false, ast.Token{}, TNil, nil}
		tok, err := lexer.scanner.Scan(lexer)
		if err != nil {
			t.Errorf("Scan(%q) error: %v", tt.input, err)
			continue
		}
		if tok.Type != tt.expected {
			t.Errorf("Scan(%q) type = %d, want %d", tt.input, tok.Type, tt.expected)
		}
	}
}

func TestScanDotVariants(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{".", '.'},
		{"..", T2Comma},
		{"...", T3Comma},
		{".5", TNumber},
	}
	for _, tt := range tests {
		lexer := &Lexer{NewScanner(strings.NewReader(tt.input), "test"), nil, false, ast.Token{}, TNil, nil}
		tok, err := lexer.scanner.Scan(lexer)
		if err != nil {
			t.Errorf("Scan(%q) error: %v", tt.input, err)
			continue
		}
		if tok.Type != tt.expected {
			t.Errorf("Scan(%q) type = %d, want %d", tt.input, tok.Type, tt.expected)
		}
	}
}
