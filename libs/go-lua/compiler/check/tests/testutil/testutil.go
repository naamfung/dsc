package testutil

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/hooks"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/stdlib"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// Config holds configuration for creating a test checker.
type Config struct {
	Stdlib    bool
	Manifests map[string]*io.Manifest
	Database  *db.DB
	Types     map[string]typ.Type
}

// Option configures a test checker.
type Option func(*Config)

// WithStdlib enables standard library in the checker.
func WithStdlib() Option {
	return func(c *Config) {
		c.Stdlib = true
	}
}

// WithManifest adds a module manifest.
func WithManifest(path string, manifest *io.Manifest) Option {
	return func(c *Config) {
		if c.Manifests == nil {
			c.Manifests = make(map[string]*io.Manifest)
		}
		c.Manifests[path] = manifest
	}
}

// WithTypes adds named types to the type scope.
func WithTypes(types map[string]typ.Type) Option {
	return func(c *Config) {
		if c.Types == nil {
			c.Types = make(map[string]typ.Type)
		}
		for name, t := range types {
			c.Types[name] = t
		}
	}
}

// NewChecker creates a checker configured for testing.
func NewChecker(opts ...Option) *check.Checker {
	cfg := &Config{
		Database: db.New(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	for path, manifest := range cfg.Manifests {
		cfg.Database.Connect(path, manifest)
	}

	var stdlibScope *scope.State
	globalTypes := make(map[string]typ.Type)

	if cfg.Stdlib {
		stdlibScope = scope.NewWithBuiltins()
		for name, t := range stdlib.Library() {
			globalTypes[name] = t
		}
	}

	for _, manifest := range cfg.Manifests {
		if stdlibScope == nil {
			stdlibScope = scope.New()
		}
		if manifest.Export != nil {
			globalTypes[manifest.Path] = manifest.Export
		}
		for name, t := range manifest.Types {
			stdlibScope = stdlibScope.WithType(name, t)
		}
		for name, t := range manifest.AllGlobals() {
			globalTypes[name] = t
		}
	}

	for name, t := range cfg.Types {
		if stdlibScope == nil {
			stdlibScope = scope.New()
		}
		stdlibScope = stdlibScope.WithType(name, t)
	}

	var engine *core.Engine
	if cfg.Stdlib {
		engine = core.NewEngineWithStdlib(stdlib.EngineConfig())
	} else {
		engine = core.NewEngine()
	}

	return check.NewChecker(cfg.Database, check.Deps{
		Types:       engine,
		Stdlib:      stdlibScope,
		GlobalTypes: globalTypes,
		Resolver: &core.FuncResolver{
			FieldFunc: core.Field,
			IndexFunc: core.Index,
		},
	}, hooks.All()...)
}

// Result holds the result of a check operation.
type Result struct {
	Session     *check.Session
	Diagnostics []diag.Diagnostic
	Errors      []diag.Diagnostic
}

// HasError returns true if there are error-level diagnostics.
func (r *Result) HasError() bool {
	return len(r.Errors) > 0
}

// Check runs the checker on source code.
func Check(source string, opts ...Option) *Result {
	checker := NewChecker(opts...)
	sess := checker.Check(source, "test.lua")

	result := &Result{
		Session:     sess,
		Diagnostics: sess.Diagnostics,
	}

	for _, d := range sess.Diagnostics {
		if d.Severity == diag.SeverityError {
			result.Errors = append(result.Errors, d)
		}
	}

	return result
}

// ErrorMessages returns all error messages from diagnostics.
func ErrorMessages(diags []diag.Diagnostic) []string {
	var msgs []string
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			msgs = append(msgs, d.Message)
		}
	}
	return msgs
}

// Case represents a single test case for table-driven tests.
type Case struct {
	Name      string
	Code      string
	WantError bool
	Stdlib    bool
	Manifests map[string]*io.Manifest
}

// RunCases runs a slice of test cases.
func RunCases(t *testing.T, tests []Case) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			var opts []Option
			if tt.Stdlib {
				opts = append(opts, WithStdlib())
			}
			for path, manifest := range tt.Manifests {
				opts = append(opts, WithManifest(path, manifest))
			}
			result := Check(tt.Code, opts...)
			if result.HasError() != tt.WantError {
				t.Errorf("wantError=%v, gotError=%v, errors=%v",
					tt.WantError, result.HasError(), ErrorMessages(result.Diagnostics))
			}
		})
	}
}

// ModuleResult holds the result of checking and exporting a module.
type ModuleResult struct {
	Session  *check.Session
	Manifest *io.Manifest
	Errors   []diag.Diagnostic
	checker  *check.Checker
}

// HasError returns true if there are error-level diagnostics.
func (r *ModuleResult) HasError() bool {
	return len(r.Errors) > 0
}

// CheckAndExport checks a module and extracts its manifest.
func CheckAndExport(source, name string, opts ...Option) *ModuleResult {
	cfg := &Config{
		Database: db.New(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	for path, manifest := range cfg.Manifests {
		cfg.Database.Connect(path, manifest)
	}

	var stdlibScope *scope.State
	globalTypes := make(map[string]typ.Type)

	if cfg.Stdlib {
		stdlibScope = scope.NewWithBuiltins()
		for sname, t := range stdlib.Library() {
			globalTypes[sname] = t
		}
	}

	for _, manifest := range cfg.Manifests {
		if stdlibScope == nil {
			stdlibScope = scope.New()
		}
		if manifest.Export != nil {
			globalTypes[manifest.Path] = manifest.Export
		}
		for tname, t := range manifest.Types {
			stdlibScope = stdlibScope.WithType(tname, t)
		}
		for name, t := range manifest.AllGlobals() {
			globalTypes[name] = t
		}
	}

	var engine *core.Engine
	if cfg.Stdlib {
		engine = core.NewEngineWithStdlib(stdlib.EngineConfig())
	} else {
		engine = core.NewEngine()
	}

	checker := check.NewChecker(cfg.Database, check.Deps{
		Types:       engine,
		Stdlib:      stdlibScope,
		GlobalTypes: globalTypes,
		Resolver: &core.FuncResolver{
			FieldFunc: core.Field,
			IndexFunc: core.Index,
		},
	}, hooks.All()...)

	sess := checker.Check(source, name+".lua")
	manifest := sess.ExportManifest(name)

	var errors []diag.Diagnostic
	for _, d := range sess.Diagnostics {
		if d.Severity == diag.SeverityError {
			errors = append(errors, d)
		}
	}

	return &ModuleResult{
		Session:  sess,
		Manifest: manifest,
		Errors:   errors,
		checker:  checker,
	}
}

// WithModule connects a previously exported module's manifest.
func WithModule(name string, mod *ModuleResult) Option {
	return func(cfg *Config) {
		if mod != nil && mod.Manifest != nil {
			if cfg.Manifests == nil {
				cfg.Manifests = make(map[string]*io.Manifest)
			}
			cfg.Manifests[name] = mod.Manifest
		}
	}
}

// ChannelManifest creates a channel manifest with proper types and effects.
func ChannelManifest() *io.Manifest {
	m := io.NewManifest("channel")

	selectCaseType := typ.NewInterface("channel.SelectCase", nil)
	selectCaseChannel := typ.NewTypeParam("C", nil)
	selectCaseValue := typ.NewTypeParam("T", nil)
	selectCaseGeneric := typ.NewGeneric("channel.SelectCase", []*typ.TypeParam{selectCaseChannel, selectCaseValue}, selectCaseType)

	channelElem := typ.NewTypeParam("T", nil)
	channelType := typ.NewInterface("channel.Channel", []typ.Method{
		{
			Name: "case_receive",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Instantiate(selectCaseGeneric, typ.Self, channelElem)).
				Build(),
		},
		{
			Name: "receive",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(channelElem, typ.Boolean).
				Build(),
		},
	})
	channelGeneric := typ.NewGeneric("channel.Channel", []*typ.TypeParam{channelElem}, channelType)

	selectResultType := typ.NewRecord().
		Field("channel", typ.Any).
		Field("value", typ.Unknown).
		Field("ok", typ.Boolean).
		Build()

	m.DefineType("Channel", channelGeneric)
	m.DefineType("SelectCase", selectCaseGeneric)
	m.DefineType("SelectResult", selectResultType)

	selectFunc := typ.Func().
		Param("cases", typ.Any).
		Returns(selectResultType).
		Spec(contract.NewSpec().WithEffectRow(effect.Returns(0, effect.SelectResultOfCases{
			Cases:   effect.ParamRef{Index: 0},
			Default: effect.ParamRef{Index: -1},
		}))).
		Build()

	moduleType := typ.NewInterface("channel", []typ.Method{
		{Name: "select", Type: selectFunc},
	})
	m.SetExport(moduleType)
	return m
}

// FuncsManifest creates a minimal funcs manifest for tests.
func FuncsManifest() *io.Manifest {
	m := io.NewManifest("funcs")

	moduleType := typ.NewInterface("funcs", []typ.Method{
		{
			Name: "call",
			Type: typ.Func().
				Param("name", typ.String).
				Variadic(typ.Any).
				Returns(typ.Any, typ.NewOptional(typ.LuaError)).
				Build(),
		},
	})

	m.SetExport(moduleType)
	return m
}

// ProcessManifest creates a process module manifest.
func ProcessManifest(channelGeneric *typ.Generic, eventType typ.Type) *io.Manifest {
	m := io.NewManifest("process")
	eventChannelType := typ.Instantiate(channelGeneric, eventType)
	moduleType := typ.NewInterface("process", []typ.Method{
		{Name: "events", Type: typ.Func().Returns(eventChannelType).Build()},
	})
	m.SetExport(moduleType)
	m.DefineType("Event", eventType)
	return m
}

// TimeManifest creates a time module manifest.
func TimeManifest(channelGeneric *typ.Generic, timeType typ.Type) *io.Manifest {
	m := io.NewManifest("time")
	timeChannelType := typ.Instantiate(channelGeneric, timeType)
	moduleType := typ.NewInterface("time", []typ.Method{
		{Name: "after", Type: typ.Func().Param("d", typ.Any).Returns(timeChannelType).Build()},
	})
	m.SetExport(moduleType)
	m.DefineType("Time", timeType)
	return m
}
