// Package validate provides validation for LValue types.
// Validators register via init() for core-based extensibility.
package validate

import (
	"regexp"
	"sync"
)

// Func validates a value against an annotation argument.
// Val is always an LValue from the lua package.
type Func func(val any, arg any) *Error

// Error represents a validation failure.
type Error struct {
	Field      string
	Message    string
	Got        any
	Expected   any
	Constraint string
}

func (e *Error) Error() string {
	if e.Field != "" {
		return e.Field + ": " + e.Message
	}
	return e.Message
}

func (e *Error) String() string {
	return e.Message
}

// Registry holds validators.
type Registry struct {
	mu         sync.RWMutex
	validators map[string]Func
}

// New creates empty registry.
func New() *Registry {
	return &Registry{validators: make(map[string]Func)}
}

// Default is the global registry with built-in validators.
var Default = New()

// Register adds a validator to the default context.
func Register(name string, fn Func) {
	Default.mu.Lock()
	Default.validators[name] = fn
	Default.mu.Unlock()
}

// RegisterValidator adds a validator to this registry.
func (r *Registry) RegisterValidator(name string, fn Func) {
	r.mu.Lock()
	r.validators[name] = fn
	r.mu.Unlock()
}

// Get returns a validator by name.
func (r *Registry) Get(name string) Func {
	r.mu.RLock()
	fn := r.validators[name]
	r.mu.RUnlock()
	return fn
}

// CheckAnnotations runs validators for annotations.
func (r *Registry) CheckAnnotations(val any, annotations map[string]any, path string, errors *[]*Error) {
	if annotations == nil {
		return
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for name, arg := range annotations {
		if fn := r.validators[name]; fn != nil {
			if err := fn(val, arg); err != nil {
				err.Field = path
				*errors = append(*errors, err)
			}
		}
	}
}

// Regex cache with size limit
const maxRegexCache = 1024

var (
	regexCache   = make(map[string]*regexp.Regexp)
	regexCacheMu sync.RWMutex
	invalidRegex = &regexp.Regexp{} // sentinel for invalid patterns
)

// GetRegex returns cached compiled regex. Returns nil for invalid patterns.
func GetRegex(pattern string) *regexp.Regexp {
	regexCacheMu.RLock()
	re, ok := regexCache[pattern]
	regexCacheMu.RUnlock()
	if ok {
		if re == invalidRegex {
			return nil
		}
		return re
	}

	compiled, err := regexp.Compile(pattern)

	regexCacheMu.Lock()
	// Double-check after acquiring write lock
	if existing, ok := regexCache[pattern]; ok {
		regexCacheMu.Unlock()
		if existing == invalidRegex {
			return nil
		}
		return existing
	}
	// Evict if cache full (simple clear strategy)
	if len(regexCache) >= maxRegexCache {
		regexCache = make(map[string]*regexp.Regexp)
	}
	if err != nil {
		regexCache[pattern] = invalidRegex
		regexCacheMu.Unlock()
		return nil
	}
	regexCache[pattern] = compiled
	regexCacheMu.Unlock()
	return compiled
}
