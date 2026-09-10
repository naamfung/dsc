package lua

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Suite describes a fixture suite loaded from manifest.json.
type fixtureSuite struct {
	Description string        `json:"description,omitempty"`
	Files       []string      `json:"files,omitempty"`
	Stdlib      *bool         `json:"stdlib,omitempty"`
	Packages    []string      `json:"packages,omitempty"` // predefined system packages: "channel", "process", "time", "funcs"
	Check       *fixtureCheck `json:"check,omitempty"`
	Run         *fixtureRun   `json:"run,omitempty"`
	Bench       *fixtureBench `json:"bench,omitempty"`
	Skip        string        `json:"skip,omitempty"`
}

type fixtureCheck struct {
	Errors *int   `json:"errors,omitempty"`
	Skip   string `json:"skip,omitempty"`
}

type fixtureRun struct {
	Golden        string `json:"golden,omitempty"`
	Error         bool   `json:"error,omitempty"`
	ErrorContains string `json:"error_contains,omitempty"`
	Skip          string `json:"skip,omitempty"`
}

type fixtureBench struct {
	Skip string `json:"skip,omitempty"`
}

type namedSuite struct {
	Name  string // path-based name for t.Run (e.g. "narrowing/typeof-guard")
	Dir   string // absolute directory path
	Suite fixtureSuite
}

type inlineExpectation struct {
	File     string
	Line     int
	Severity string // "error" or "warning"
	Contains string
}

var expectRe = regexp.MustCompile(`--\s*expect-(error|warning)(?::\s*(.+?))?\s*$`)

// discoverFixtures recursively walks root and finds directories containing .lua files.
func discoverFixtures(root string) ([]namedSuite, error) {
	var suites []namedSuite
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		luaFiles, _ := filepath.Glob(filepath.Join(path, "*.lua"))
		if len(luaFiles) == 0 {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		name := filepath.ToSlash(rel)

		s := fixtureSuite{}
		manifestPath := filepath.Join(path, "manifest.json")
		if data, err := os.ReadFile(manifestPath); err == nil {
			if err := json.Unmarshal(data, &s); err != nil {
				return fmt.Errorf("bad manifest in %s: %w", name, err)
			}
		}

		suites = append(suites, namedSuite{Name: name, Dir: path, Suite: s})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(suites, func(i, j int) bool { return suites[i].Name < suites[j].Name })
	return suites, nil
}

// resolveFiles returns the ordered file list for the suite.
func resolveFiles(s namedSuite) []string {
	if len(s.Suite.Files) > 0 {
		return s.Suite.Files
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return []string{"main.lua"}
	}
	var modules []string
	hasMain := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".lua") {
			continue
		}
		if e.Name() == "main.lua" {
			hasMain = true
			continue
		}
		modules = append(modules, e.Name())
	}
	sort.Strings(modules)
	if hasMain {
		return append(modules, "main.lua")
	}
	if len(modules) > 0 {
		return modules
	}
	return []string{"main.lua"}
}

func resolveStdlib(s namedSuite) bool {
	if s.Suite.Stdlib != nil {
		return *s.Suite.Stdlib
	}
	return true
}

func readFixtureFile(dir, name string) string {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		panic(fmt.Sprintf("fixture file %s/%s: %v", dir, name, err))
	}
	return string(data)
}

// parseExpectations scans source lines for expect-error/expect-warning comments.
func parseExpectations(filename, source string) []inlineExpectation {
	var expectations []inlineExpectation
	for i, line := range strings.Split(source, "\n") {
		m := expectRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		expectations = append(expectations, inlineExpectation{
			File:     filename,
			Line:     i + 1,
			Severity: m[1],
			Contains: strings.TrimSpace(m[2]),
		})
	}
	return expectations
}

// runCheckPhase type-checks the fixture and verifies diagnostics.
func runCheckPhase(t *testing.T, s namedSuite) {
	t.Helper()
	if s.Suite.Check != nil && s.Suite.Check.Skip != "" {
		t.Skip(s.Suite.Check.Skip)
	}

	files := resolveFiles(s)
	stdlib := resolveStdlib(s)

	var baseOpts []testutil.Option
	if stdlib {
		baseOpts = append(baseOpts, testutil.WithStdlib())
	}
	for _, pkg := range s.Suite.Packages {
		if m := resolvePackageManifest(pkg); m != nil {
			baseOpts = append(baseOpts, testutil.WithManifest(pkg, m))
		} else {
			t.Fatalf("unknown system package: %s", pkg)
		}
	}

	// Collect all sources and their expectations
	sources := make(map[string]string)
	var allExpectations []inlineExpectation
	for _, f := range files {
		src := readFixtureFile(s.Dir, f)
		sources[f] = src
		allExpectations = append(allExpectations, parseExpectations(f, src)...)
	}

	// Check and export dependency modules (all except entry), preserving file order
	type namedModule struct {
		name string
		mod  *testutil.ModuleResult
	}
	var moduleOrder []namedModule
	var allDiagnostics []diag.Diagnostic
	for _, f := range files[:len(files)-1] {
		modOpts := append([]testutil.Option{}, baseOpts...)
		for _, nm := range moduleOrder {
			modOpts = append(modOpts, testutil.WithModule(nm.name, nm.mod))
		}
		name := strings.TrimSuffix(f, ".lua")
		mod := testutil.CheckAndExport(sources[f], name, modOpts...)
		moduleOrder = append(moduleOrder, namedModule{name, mod})
		allDiagnostics = append(allDiagnostics, mod.Errors...)
	}

	// Check entry point
	entryOpts := append([]testutil.Option{}, baseOpts...)
	for _, nm := range moduleOrder {
		entryOpts = append(entryOpts, testutil.WithModule(nm.name, nm.mod))
	}
	entryFile := files[len(files)-1]
	result := testutil.Check(sources[entryFile], entryOpts...)
	allDiagnostics = append(allDiagnostics, result.Diagnostics...)

	// Verify expectations
	if len(allExpectations) > 0 {
		verifyInlineExpectations(t, allExpectations, allDiagnostics, entryFile)
	} else if s.Suite.Check != nil && s.Suite.Check.Errors != nil {
		verifyErrorCount(t, *s.Suite.Check.Errors, allDiagnostics)
	} else {
		verifyClean(t, allDiagnostics)
	}
}

func verifyInlineExpectations(t *testing.T, expectations []inlineExpectation, diagnostics []diag.Diagnostic, entryFile string) {
	t.Helper()
	matched := make([]bool, len(diagnostics))
	failed := false

	for _, exp := range expectations {
		found := false
		// An expect-error annotation absorbs ALL matching diagnostics on that line
		for i, d := range diagnostics {
			if !matchesExpectation(exp, d, entryFile) {
				continue
			}
			found = true
			matched[i] = true
		}
		if !found {
			failed = true
			if exp.Contains != "" {
				t.Errorf("expected %s at %s:%d not found: %q", exp.Severity, exp.File, exp.Line, exp.Contains)
			} else {
				t.Errorf("expected %s at %s:%d not found", exp.Severity, exp.File, exp.Line)
			}
		}
	}

	for i, d := range diagnostics {
		if matched[i] || d.Severity == diag.SeverityHint {
			continue
		}
		failed = true
		t.Errorf("unexpected %s at %s:%d: %s (%s)",
			d.Severity, d.Position.File, d.Position.Line, d.Message, d.Code.Name())
	}

	if failed {
		dumpDiagnostics(t, diagnostics)
	}
}

func matchesExpectation(exp inlineExpectation, d diag.Diagnostic, entryFile string) bool {
	expFile := exp.File
	// Match diagnostic file: d.Position.File is set by the checker (e.g. "test.lua" or module name)
	if !strings.HasSuffix(d.Position.File, strings.TrimSuffix(expFile, ".lua")) &&
		(expFile != entryFile || d.Position.File != "test.lua") {
		return false
	}
	if d.Position.Line != exp.Line {
		return false
	}
	wantSeverity := diag.SeverityError
	if exp.Severity == "warning" {
		wantSeverity = diag.SeverityWarning
	}
	if d.Severity != wantSeverity {
		return false
	}
	if exp.Contains != "" && !strings.Contains(d.Message, exp.Contains) {
		return false
	}
	return true
}

func verifyErrorCount(t *testing.T, want int, diagnostics []diag.Diagnostic) {
	t.Helper()
	var errors []diag.Diagnostic
	for _, d := range diagnostics {
		if d.Severity == diag.SeverityError {
			errors = append(errors, d)
		}
	}
	if len(errors) != want {
		t.Errorf("expected %d errors, got %d", want, len(errors))
		dumpDiagnostics(t, diagnostics)
	}
}

func verifyClean(t *testing.T, diagnostics []diag.Diagnostic) {
	t.Helper()
	var errors []diag.Diagnostic
	for _, d := range diagnostics {
		if d.Severity == diag.SeverityError {
			errors = append(errors, d)
		}
	}
	if len(errors) > 0 {
		t.Errorf("expected clean check, got %d errors", len(errors))
		dumpDiagnostics(t, diagnostics)
	}
}

func dumpDiagnostics(t *testing.T, diagnostics []diag.Diagnostic) {
	t.Helper()
	t.Log("--- all diagnostics ---")
	for _, d := range diagnostics {
		t.Logf("  %s:%d:%d [%s] %s: %s",
			d.Position.File, d.Position.Line, d.Position.Column,
			d.Severity, d.Code.Name(), d.Message)
	}
}

// runExecPhase executes the fixture and verifies output.
func runExecPhase(t *testing.T, s namedSuite) {
	t.Helper()
	if s.Suite.Run == nil {
		// Auto-enable if output.golden exists
		goldenPath := filepath.Join(s.Dir, "output.golden")
		if _, err := os.Stat(goldenPath); err != nil {
			return
		}
	} else if s.Suite.Run.Skip != "" {
		t.Skip(s.Suite.Run.Skip)
	}

	files := resolveFiles(s)

	L := NewState()
	defer L.Close()
	OpenBase(L)
	OpenString(L)
	OpenTable(L)
	OpenMath(L)

	// Build module source map and install require
	moduleSources := make(map[string]string)
	for _, f := range files[:len(files)-1] {
		moduleSources[strings.TrimSuffix(f, ".lua")] = readFixtureFile(s.Dir, f)
	}
	installRequire(L, moduleSources)

	// Capture print output
	var buf bytes.Buffer
	capturePrint(L, &buf)

	// Execute entry point
	entrySrc := readFixtureFile(s.Dir, files[len(files)-1])
	err := L.DoString(entrySrc)

	runCfg := s.Suite.Run
	if runCfg != nil && runCfg.Error {
		if err == nil {
			t.Error("expected runtime error, got none")
		} else if runCfg.ErrorContains != "" && !strings.Contains(err.Error(), runCfg.ErrorContains) {
			t.Errorf("error %q does not contain %q", err.Error(), runCfg.ErrorContains)
		}
		return
	}
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}

	verifyGoldenOutput(t, s, &buf)
}

// resolvePackageManifest returns a predefined manifest for a system package name.
func resolvePackageManifest(name string) *io.Manifest {
	switch name {
	case "channel":
		return testutil.ChannelManifest()
	case "funcs":
		return testutil.FuncsManifest()
	case "time":
		return fixtureTimeManifest()
	default:
		return nil
	}
}

func fixtureTimeManifest() *io.Manifest {
	m := io.NewManifest("time")

	durationType := typ.NewInterface("time.Duration", []typ.Method{
		{Name: "seconds", Type: typ.Func().Param("self", typ.Self).Returns(typ.Number).Build()},
	})

	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "sub", Type: typ.Func().Param("self", typ.Self).Param("t", typ.Self).Returns(durationType).Build()},
		{Name: "add", Type: typ.Func().Param("self", typ.Self).Param("d", durationType).Returns(typ.Self).Build()},
		{Name: "unix", Type: typ.Func().Param("self", typ.Self).Returns(typ.Integer).Build()},
	})

	m.DefineType("Time", timeType)
	m.DefineType("Duration", durationType)

	moduleType := typ.NewInterface("time", []typ.Method{
		{Name: "now", Type: typ.Func().Returns(timeType).Build()},
	})
	m.SetExport(moduleType)

	return m
}

// installRequire sets up a require() global that loads modules from the given source map.
// Modules are compiled, executed, cached, and returned — matching standard Lua require semantics.
func installRequire(L *LState, sources map[string]string) {
	loaded := L.NewTable()
	L.SetGlobal("require", L.NewFunction(func(L *LState) int {
		name := L.CheckString(1)
		// Return cached module
		if cached := loaded.RawGetString(name); cached != LNil {
			L.Push(cached)
			return 1
		}
		src, ok := sources[name]
		if !ok {
			L.RaiseError("module '%s' not found", name)
			return 0
		}
		fn, err := L.LoadString(src)
		if err != nil {
			L.RaiseError("module '%s': %s", name, err.Error())
			return 0
		}
		L.Push(fn)
		L.Call(0, 1)
		result := L.Get(-1)
		if result == LNil {
			result = LTrue
		}
		loaded.RawSetString(name, result)
		return 1
	}))
}

func capturePrint(L *LState, buf *bytes.Buffer) {
	L.SetGlobal("print", L.NewFunction(func(L *LState) int {
		top := L.GetTop()
		for i := 1; i <= top; i++ {
			if i > 1 {
				buf.WriteByte('\t')
			}
			buf.WriteString(L.ToStringMeta(L.Get(i)).String())
		}
		buf.WriteByte('\n')
		return 0
	}))
}

func verifyGoldenOutput(t *testing.T, s namedSuite, buf *bytes.Buffer) {
	t.Helper()
	goldenName := "output.golden"
	if s.Suite.Run != nil && s.Suite.Run.Golden != "" {
		goldenName = s.Suite.Run.Golden
	}
	goldenPath := filepath.Join(s.Dir, goldenName)

	if os.Getenv("FIXTURE_UPDATE") != "" {
		got := buf.String()
		if got != "" {
			if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
				t.Fatalf("updating golden file: %v", err)
			}
			t.Logf("updated %s", goldenPath)
		}
		return
	}

	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) && buf.Len() == 0 {
			return // no golden file and no output, that's fine
		}
		if os.IsNotExist(err) {
			t.Fatalf("output produced but no golden file at %s (run with FIXTURE_UPDATE=1 to create)", goldenPath)
		}
		t.Fatalf("reading golden file: %v", err)
	}

	got := buf.String()
	want := string(golden)
	if got != want {
		t.Errorf("output mismatch:\n--- want ---\n%s--- got ---\n%s", want, got)
	}
}

// runBenchPhase benchmarks the fixture.
func runBenchPhase(b *testing.B, s namedSuite) {
	b.Helper()
	if s.Suite.Bench == nil {
		b.Skip("no bench config")
	}
	if s.Suite.Bench.Skip != "" {
		b.Skip(s.Suite.Bench.Skip)
	}

	files := resolveFiles(s)

	L := NewState()
	defer L.Close()
	OpenBase(L)
	OpenString(L)
	OpenTable(L)
	OpenMath(L)

	moduleSources := make(map[string]string)
	for _, f := range files[:len(files)-1] {
		moduleSources[strings.TrimSuffix(f, ".lua")] = readFixtureFile(s.Dir, f)
	}
	installRequire(L, moduleSources)

	// Silence print
	L.SetGlobal("print", L.NewFunction(func(L *LState) int { return 0 }))

	entrySrc := readFixtureFile(s.Dir, files[len(files)-1])
	fn, err := L.LoadString(entrySrc)
	if err != nil {
		b.Fatalf("compile error: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		if err := L.PCall(0, MultRet, nil); err != nil {
			b.Fatalf("runtime error: %v", err)
		}
		L.SetTop(0)
	}
}
