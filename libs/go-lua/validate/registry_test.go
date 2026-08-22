package validate

import (
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	ctx := New()
	if ctx == nil {
		t.Fatal("New() returned nil")
	}
	if ctx.validators == nil {
		t.Error("validators map not initialized")
	}
}

func TestRegisterAndGet(t *testing.T) {
	ctx := New()

	called := false
	ctx.RegisterValidator("test", func(val any, arg any) *Error {
		called = true
		return nil
	})

	fn := ctx.Get("test")
	if fn == nil {
		t.Fatal("expected validator")
	}

	_ = fn("value", "arg")
	if !called {
		t.Error("validator not called")
	}
}

func TestGetNotFound(t *testing.T) {
	ctx := New()
	fn := ctx.Get("nonexistent")
	if fn != nil {
		t.Error("expected nil for non-existent")
	}
}

func TestOverwrite(t *testing.T) {
	ctx := New()
	ctx.RegisterValidator("test", func(val any, arg any) *Error {
		return &Error{Message: "first"}
	})
	ctx.RegisterValidator("test", func(val any, arg any) *Error {
		return &Error{Message: "second"}
	})

	fn := ctx.Get("test")
	err := fn("val", "arg")
	if err.Message != "second" {
		t.Errorf("message = %q, want 'second'", err.Message)
	}
}

func TestConcurrentAccess(t *testing.T) {
	ctx := New()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			name := string(rune('a' + n%26))
			ctx.RegisterValidator(name, func(val any, arg any) *Error { return nil })
		}(i)
		go func(n int) {
			defer wg.Done()
			name := string(rune('a' + n%26))
			ctx.Get(name)
		}(i)
	}

	wg.Wait()
}

func TestCheckAnnotations(t *testing.T) {
	ctx := New()
	ctx.RegisterValidator("positive", func(val any, arg any) *Error {
		if n, ok := val.(int); ok && n < 0 {
			return &Error{Message: "must be positive", Constraint: "positive"}
		}
		return nil
	})

	tests := []struct {
		name        string
		val         any
		annotations map[string]any
		wantErrors  int
	}{
		{"nil annotations", 5, nil, 0},
		{"empty annotations", 5, map[string]any{}, 0},
		{"valid", 5, map[string]any{"positive": true}, 0},
		{"invalid", -1, map[string]any{"positive": true}, 1},
		{"unknown skipped", 5, map[string]any{"unknown": true}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errors []*Error
			ctx.CheckAnnotations(tt.val, tt.annotations, "field", &errors)
			if len(errors) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(errors), tt.wantErrors)
			}
			for _, err := range errors {
				if err.Field != "field" {
					t.Errorf("field = %q, want 'field'", err.Field)
				}
			}
		})
	}
}

func TestDefaultContext(t *testing.T) {
	validators := []string{"min", "max", "min_len", "max_len", "pattern"}
	for _, name := range validators {
		if Default.Get(name) == nil {
			t.Errorf("expected default validator %q", name)
		}
	}
}

func TestGlobalRegister(t *testing.T) {
	Register("custom_global", func(val any, arg any) *Error {
		return &Error{Message: "custom", Constraint: "custom_global"}
	})

	fn := Default.Get("custom_global")
	if fn == nil {
		t.Fatal("expected custom_global")
	}

	err := fn("val", "arg")
	if err.Constraint != "custom_global" {
		t.Errorf("constraint = %q", err.Constraint)
	}
}

func TestErrorString(t *testing.T) {
	tests := []struct {
		err  *Error
		want string
	}{
		{&Error{Field: "age", Message: "too low"}, "age: too low"},
		{&Error{Message: "invalid"}, "invalid"},
		{&Error{Field: "", Message: "error"}, "error"},
		{&Error{Field: "a.b.c", Message: "invalid"}, "a.b.c: invalid"},
	}

	for _, tt := range tests {
		got := tt.err.Error()
		if got != tt.want {
			t.Errorf("Error() = %q, want %q", got, tt.want)
		}
	}
}

func BenchmarkGet(b *testing.B) {
	ctx := New()
	ctx.RegisterValidator("test", func(val any, arg any) *Error { return nil })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.Get("test")
	}
}

func BenchmarkCheckAnnotationsNone(b *testing.B) {
	ctx := New()
	var errors []*Error

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		errors = errors[:0]
		ctx.CheckAnnotations(42, nil, "field", &errors)
	}
}

func BenchmarkCheckAnnotationsSingle(b *testing.B) {
	ctx := New()
	ctx.RegisterValidator("min", validateMin)
	annotations := map[string]any{"min": 10}
	var errors []*Error

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		errors = errors[:0]
		ctx.CheckAnnotations(42, annotations, "field", &errors)
	}
}

func TestCheckAnnotationsMultiple(t *testing.T) {
	ctx := New()
	ctx.RegisterValidator("min", validateMin)
	ctx.RegisterValidator("max", validateMax)

	var errors []*Error
	ctx.CheckAnnotations(5, map[string]any{"min": 0, "max": 10}, "value", &errors)
	if len(errors) != 0 {
		t.Errorf("expected no errors, got %d", len(errors))
	}

	errors = nil
	ctx.CheckAnnotations(15, map[string]any{"min": 0, "max": 10}, "value", &errors)
	if len(errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(errors))
	}

	errors = nil
	ctx.CheckAnnotations(-5, map[string]any{"min": 0, "max": 10}, "value", &errors)
	if len(errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(errors))
	}

	errors = nil
	ctx.CheckAnnotations(100, map[string]any{"min": 10, "max": 5}, "value", &errors)
	if len(errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(errors))
	}
}

func TestCheckAnnotationsNestedPath(t *testing.T) {
	ctx := New()
	ctx.RegisterValidator("min", validateMin)

	var errors []*Error
	ctx.CheckAnnotations(-1, map[string]any{"min": 0}, "user.profile.age", &errors)
	if len(errors) != 1 {
		t.Fatal("expected 1 error")
	}
	if errors[0].Field != "user.profile.age" {
		t.Errorf("Field = %q, want 'user.profile.age'", errors[0].Field)
	}
}

func TestRegistryIsolation(t *testing.T) {
	ctx1 := New()
	ctx2 := New()

	ctx1.RegisterValidator("custom", func(val any, arg any) *Error {
		return &Error{Message: "ctx1"}
	})
	ctx2.RegisterValidator("custom", func(val any, arg any) *Error {
		return &Error{Message: "ctx2"}
	})

	fn1 := ctx1.Get("custom")
	fn2 := ctx2.Get("custom")

	if fn1("", nil).Message != "ctx1" {
		t.Error("ctx1 returned wrong validator")
	}
	if fn2("", nil).Message != "ctx2" {
		t.Error("ctx2 returned wrong validator")
	}
}

func TestCheckAnnotationsAccumulateErrors(t *testing.T) {
	ctx := New()
	ctx.RegisterValidator("fail1", func(val any, arg any) *Error {
		return &Error{Message: "error1"}
	})
	ctx.RegisterValidator("fail2", func(val any, arg any) *Error {
		return &Error{Message: "error2"}
	})

	var errors []*Error
	ctx.CheckAnnotations("test", map[string]any{"fail1": true, "fail2": true}, "field", &errors)
	if len(errors) != 2 {
		t.Errorf("expected 2 errors, got %d", len(errors))
	}
}

func TestConcurrentCheckAnnotations(t *testing.T) {
	ctx := New()
	ctx.RegisterValidator("min", validateMin)

	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func(n int) {
			var errors []*Error
			ctx.CheckAnnotations(n, map[string]any{"min": 50}, "value", &errors)
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}

func BenchmarkCheckAnnotationsMultiple(b *testing.B) {
	ctx := New()
	ctx.RegisterValidator("min", validateMin)
	ctx.RegisterValidator("max", validateMax)
	annotations := map[string]any{"min": 0, "max": 100}
	var errors []*Error

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		errors = errors[:0]
		ctx.CheckAnnotations(42, annotations, "field", &errors)
	}
}

func BenchmarkRegisterValidator(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ctx := New()
		ctx.RegisterValidator("test", func(val any, arg any) *Error { return nil })
	}
}
