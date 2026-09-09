package validate

import "testing"

func TestValidatePattern(t *testing.T) {
	tests := []struct {
		name    string
		val     any
		pattern string
		wantErr bool
	}{
		{"email valid", "test@example.com", `^.+@.+\..+$`, false},
		{"email invalid", "notanemail", `^.+@.+\..+$`, true},
		{"digits valid", "12345", `^\d+$`, false},
		{"digits invalid", "123abc", `^\d+$`, true},
		{"alpha valid", "hello", `^[a-zA-Z]+$`, false},
		{"alpha invalid", "hello123", `^[a-zA-Z]+$`, true},
		{"empty vs required", "", `^.+$`, true},
		{"empty vs optional", "", `^.*$`, false},
		{"number skipped", 123, `^\d+$`, false},
		{"nil skipped", nil, `^\d+$`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePattern(tt.val, tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePattern() = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && err.Constraint != "pattern" {
				t.Errorf("constraint = %q, want 'pattern'", err.Constraint)
			}
		})
	}
}

func TestValidatePatternInvalidRegex(t *testing.T) {
	err := validatePattern("test", "[invalid")
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
	if err.Message != "invalid regex" {
		t.Errorf("message = %q, want 'invalid regex'", err.Message)
	}
}

func TestValidatePatternInvalidArg(t *testing.T) {
	err := validatePattern("test", 123)
	if err == nil {
		t.Fatal("expected error for non-string pattern")
	}
	if err.Message != "invalid pattern argument" {
		t.Errorf("message = %q", err.Message)
	}
}

func TestValidatePatternWithStringer(t *testing.T) {
	err := validatePattern(mockStr("test@example.com"), `^.+@.+\..+$`)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	err = validatePattern(mockStr("invalid"), `^.+@.+\..+$`)
	if err == nil {
		t.Error("expected error")
	}
}

func TestAsString(t *testing.T) {
	tests := []struct {
		name   string
		val    any
		want   string
		wantOk bool
	}{
		{"string", "hello", "hello", true},
		{"stringer", mockStr("test"), "test", true},
		{"int", 42, "", false},
		{"nil", nil, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := asString(tt.val)
			if ok != tt.wantOk {
				t.Errorf("asString() ok = %v, want %v", ok, tt.wantOk)
			}
			if ok && got != tt.want {
				t.Errorf("asString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetRegexCaching(t *testing.T) {
	pattern := `^test\d+$`

	re1 := GetRegex(pattern)
	if re1 == nil {
		t.Fatal("expected compiled regex")
	}

	re2 := GetRegex(pattern)
	if re2 == nil {
		t.Fatal("expected compiled regex")
	}

	if re1 != re2 {
		t.Error("expected same cached regex instance")
	}
}

func TestGetRegexInvalid(t *testing.T) {
	re := GetRegex("[invalid")
	if re != nil {
		t.Error("expected nil for invalid regex")
	}
}

func BenchmarkValidatePattern(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = validatePattern("test@example.com", `^.+@.+\..+$`)
	}
}

func BenchmarkValidatePatternComplex(b *testing.B) {
	pattern := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	for i := 0; i < b.N; i++ {
		_ = validatePattern("user.name+tag@subdomain.example.com", pattern)
	}
}

func TestError_String(t *testing.T) {
	tests := []struct {
		name    string
		err     *Error
		wantStr string
	}{
		{
			name:    "with field",
			err:     &Error{Field: "name", Message: "required"},
			wantStr: "required",
		},
		{
			name:    "without field",
			err:     &Error{Message: "validation failed"},
			wantStr: "validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.String(); got != tt.wantStr {
				t.Errorf("String() = %q, want %q", got, tt.wantStr)
			}
		})
	}
}

func TestValidatePatternComplexPatterns(t *testing.T) {
	tests := []struct {
		name    string
		val     any
		pattern string
		wantErr bool
	}{
		{"uuid valid", "550e8400-e29b-41d4-a716-446655440000", `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, false},
		{"uuid invalid", "not-a-uuid", `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, true},
		{"url valid", "https://example.com/path", `^https?://`, false},
		{"url invalid", "ftp://example.com", `^https?://`, true},
		{"phone valid", "+1-555-555-5555", `^\+\d{1,3}-\d{3}-\d{3}-\d{4}$`, false},
		{"phone invalid", "555-5555", `^\+\d{1,3}-\d{3}-\d{3}-\d{4}$`, true},
		{"alphanumeric valid", "abc123", `^[a-zA-Z0-9]+$`, false},
		{"alphanumeric with special", "abc_123", `^[a-zA-Z0-9]+$`, true},
		{"whitespace", "hello world", `^\S+$`, true},
		{"no whitespace", "helloworld", `^\S+$`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePattern(tt.val, tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePattern() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePatternErrorFields(t *testing.T) {
	err := validatePattern("abc", `^\d+$`)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Got != "abc" {
		t.Errorf("Got = %v, want 'abc'", err.Got)
	}
	if err.Expected != `^\d+$` {
		t.Errorf("Expected = %v", err.Expected)
	}
}

func TestAsStringEmptyString(t *testing.T) {
	got, ok := asString("")
	if !ok {
		t.Error("expected ok for empty string")
	}
	if got != "" {
		t.Errorf("got = %q, want empty", got)
	}
}

func TestValidatePatternEmptyPattern(t *testing.T) {
	err := validatePattern("anything", "")
	if err != nil {
		t.Error("expected no error for empty pattern")
	}
}

func TestValidatePatternAnchoredPatterns(t *testing.T) {
	err := validatePattern("prefix_test", "^prefix")
	if err != nil {
		t.Error("expected match for prefix pattern")
	}

	err = validatePattern("test_suffix", "suffix$")
	if err != nil {
		t.Error("expected match for suffix pattern")
	}

	err = validatePattern("test", "^prefix")
	if err == nil {
		t.Error("expected no match for prefix pattern")
	}
}

func TestGetRegexConcurrency(t *testing.T) {
	patterns := []string{`^\d+$`, `^[a-z]+$`, `^[A-Z]+$`, `^.+@.+$`}

	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func(n int) {
			pattern := patterns[n%len(patterns)]
			re := GetRegex(pattern)
			if re == nil {
				t.Errorf("expected valid regex for %s", pattern)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}
