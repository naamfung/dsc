# lisp

Derived from `kanaka/mal` Go implementation of a Lisp interpreter.
`kanaka/mal` Lisp is _Clojure inspired_.

Keeping 100% backwards compatibility with `kanaka/mal`.
There almost 100 implementations on almost 100 languages available on repository [kanaka/mal](https://github.com/kanaka/mal).

This derived implementation is focused on _embeddability_ in Go projects.
See [lisp main](./cmd/lisp) for an example on how to embed it in Go code.

Requires Go 1.25.

This implementation uses [chzyer/readline](https://github.com/chzyer/readline) instead of C implented readline or libedit, making this implementation pure Go.

## Breaking Changes

### Position Tracking Improvements (2026-01-15)

**Breaking API Changes:**

1. **Constructor signatures changed** for programmatically created data structures:
   - `types.NewList(cursor *Position, a ...MalType)` - now requires cursor as first parameter
   - `types.NewHashMap(cursor *Position, seq MalType)` - now requires cursor as first parameter
   - Pass `nil` for cursor if position tracking is not needed

2. **Error format changed**:
   - `LispError.Error()` now includes stack trace with multiple lines
   - Format: `error_message\n  at position1\n  at position2\n...`
   - Code that parses error messages should use `strings.Contains()` instead of `strings.HasSuffix()`

3. **LispError struct changed**:
   - Added `Stack []*Position` field for call stack tracking
   - `NewLispError()` now preserves original cursor instead of overwriting it

**Migration Guide:**

```go
// Before:
list := types.NewList(elem1, elem2, elem3)
hm, _ := types.NewHashMap(listOfKeyValues)

// After:
list := types.NewList(nil, elem1, elem2, elem3)  // nil if no position tracking needed
hm, _ := types.NewHashMap(nil, listOfKeyValues)

// Or with position tracking:
list := types.NewList(currentCursor, elem1, elem2, elem3)
hm, _ := types.NewHashMap(currentCursor, listOfKeyValues)
```

**Benefits:**
- Improved error messages with full stack traces
- Better debugging of nested function calls and macro expansions
- Preserved position information through try/catch blocks
- More accurate line numbers in quasiquote and generated code

# Changes

Changes respect to [kanaka/mal](https://github.com/kanaka/mal):

- Using `def` instead of `def!`, `try` instead of `try*`, etc. symbols
- `atom` is multithread
- Tests executed using Go test library. Original implementation uses a `runtest.py` in Python to keep all implementations compatible. But it makes the Go development less enjoyable. Tests files are the original ones, there is simply a new `runtest_test.go` that substitutes the original Python script
- Some tests are actually in lisp (mal), using the macros commented in _Additions_ section (now only the test library itself). Well, actually not many at this moment, see "Test file specs" below
- Reader regexp's are removed and substituted by an ad-hoc scanner [jig/scanner](https://github.com/jig/scanner)
- `core` library moved to `lib/core`
- Using [chzyer/readline](https://github.com/chzyer/readline) instead of C `readline` for the mal REPL
- Multiline REPL
- REPL history stored in `~/.lisp_history` (instead of kanaka/mal's `~/.mal-history`)
- `(let () A B C)` returns `C` as Clojure `let` instead of `A`, and evaluates `A`, `B` and `C`
- `(do)` returns nil as Clojure instead of panicking
- `hash-map` creates maps or converts a Go object to a map if the marshaler is defined in Go for that object
- `reduce-kv` added
- `take`, `take-last`, `drop`, `drop-last`, `subvec` added

To test the implementation use:

```bash
go test ./...
```

`go test` actually validates the `step*.mal` files.

There are some benchmarks as well:

```bash
go test -benchmem -benchtime 5s -bench '^.+$' github.com/jig/lisp
```

# Additions

- Errors return line position and stack trace
- `(range a b)` returns a vector of integers from `a` to `b-1`
- `(merge hm1 hm2)` returns the merge of two hash maps, second takes precedence
- `(unbase64 string)`, `(unbase64 byteString)`, `(str2binary string)`, `(binary2str byteString)` to deal with `[]byte` variables
- `(sleep ms)` sleeps `ms` milliseconds
- Support of `¬` as string terminator to simplify JSON strings. Strings that start with `{"` and end with `"}` are printed using `¬`, otherwise strings are printed as usual (with `"`). To escape a `¬` character in a `¬` delimited string you must escape it by doubling it: `¬Hello¬¬World!¬` would be printed as `Hello¬World`. This behaviour allows to not to have to escape `"` nor `\` characters
- `(json-decode {} ¬{"key": "value"}¬)` to decode JSON to lisp hash map
- `(json-encode obj)` JSON encodes either a lisp structure or a go. Example: `(json-encode (json-decode {} ¬{"key":"value","key1": [{"a":"b","c":"d"},2,3]}¬))`. Note that lisp vectors (e.g. `[1 2 3]`) and lisp lists (e.g. `(list 1 2 3)` are both converted to JSON vectors always. Decoding a JSON vector is done on a lisp vector always though
- `(hash-map-decode (new-go-object) ¬{"key": "value"}¬)` to decode hash map to a Go struct if that struct has the appropiate Go marshaler
- `(context (do ...))` provides a Go context. Context contents depend on Go, and might be passed to specific functions context compatible
- Test minimal library to be used with `maltest` interpreter (see [./cmd/maltest/](./cmd/maltest/) folder). See below test specs
- Project compatible with GitHub CodeSpaces. Press `.` on your keyboard and you are ready to deploy a CodeSpace with mal in it
- `(assert expr & optional-error)` asserts expression is not `nil` nor `false`, otherwise it success returning `nil`
- Errors are decorated with line numbers
- `(rename-keys hm hmAlterKeys)` as in Clojure
- `(get-in m ks)` to access nested values from a `m` map; `ks` must be a vector of hash map keys
- `(uuid)` returns an 128 bit rfc4122 random UUID
- `(split string cutset)` returns a lisp Vector of the elements splitted by the cutset (see [./tests/stepH_strings](./tests/stepH_strings.mal) for examples)
- support of (hashed, unordered) sets. Only sets of strings or keywords supported. Use `#{}` for literal sets. Functions supported for sets: `set`, `set?`, `conj`, `get`, `assoc`, `dissoc`, `contains?`, `empty?`. `meta`, `with-meta` (see [./tests/stepA_mal](./tests/stepA_set.mal) and [./tests/stepF_mal](./tests/stepF_set.mal) for examples). `json-encode` will encode a set to a JSON array
- `update`, `update-in` and `assoc-in` supported for hash maps and vectors
- Go function `READ_WithPreamble` works like `READ` but supports placeholders to be filled on READ time (see [./placeholder_test.go](./placeholder_test.go) for som samples)
- Added support for `finally` inside `try`. `finally` expression is evaluated for side effects only. `finally` is optional
- Added `spew`
- Added `future`, and `future-*` companion functions from Clojure
- `type?` returns the type name string
- `go-error`, `unwrap` and `panic` mapping to Go's `errors.New/fmt.Errorf`, `Unwrap` and `panic` respectively
- `getenv`, `setenv` and `unsetenv` functions for environment variables
- `defn`, `wait` macros added (see [./tests/stepN_defn.mal.go](./tests/stepN_defn.mal) for an example of `defn` and `wait` macro usage, or go to Clojure documentation)
- `partial` function added (see [./tests/stepN_defn.mal.go](./tests/stepN_defn.mal) for an example of `partial` usage, or go to Clojure documentation)


# Embed Lisp in Go code

You execute lisp from Go code and get results from it back to Go. Example from [./example_test/example_test.go](./example_test/example_test.go):

```go
func ExampleEVAL() {
    newEnv := env.NewEnv()

    // Load required lisp libraries
    for _, library := range []struct {
        name string
        load func(newEnv types.EnvType) error
    }{
        {"core mal", nscore.Load},
        {"core mal with input", nscore.LoadInput},
        {"command line args", nscore.LoadCmdLineArgs},
        {"concurrent", nsconcurrent.Load},
        {"core mal extended", nscoreextended.Load},
        {"system", nssystem.Load},
    } {
        if err := library.load(newEnv); err != nil {
            log.Fatalf("Library Load Error %q: %v", library.name, err)
        }
    }

    // parse (READ) lisp code
    ast, err := lisp.READ(`(+ 2 2)`, nil, newEnv)
    if err != nil {
        log.Fatalf("READ error: %v", err)
    }

    // eval AST
    result, err := lisp.EVAL(context.TODO(), ast, newEnv)
    if err != nil {
        log.Fatalf("EVAL error: %v", err)
    }

    // use result
    if result.(int) != 4 {
        log.Fatalf("Result check error: %v", err)
    }

    // optionally print resulting AST
    fmt.Println(lisp.PRINT(result))
    // Output: 4
}
```

# L notation

You may generate lisp Go structures without having to parse lisp strings, by using Go `L` notation.

```go
var (
    prn = S("prn")
    str = S("str")
)

// (prn (str "hello" " " "world!"))
sampleCode := L(prn, L(str, "hello", " ", "world!"))

EVAL(sampleCode, newTestEnv(), nil)
```

See [./helloworldlnotationexample_test.go](./helloworldlnotationexample_test.go) and [./lnotation/lnotation_test.go](./lnotation/lnotation_test.go).

# Test file specs

Execute the testfile with:

```bash
$ lisp --test .
```

And a minimal test example `sample_test.mal`:

```clojure
(test.suite "complete tests"
    (assert-true "2 + 2 = 4 is true" (= 4 (+ 2 2)))
    (assert-false "2 + 2 = 5 is false" (= 5 (+ 2 2)))
    (assert-throws "0 / 0 throws an error" (/ 0 0)))
```

Some benchmark of the implementations:

```bash
$ go test -bench ".+" -benchtime 2s
```

# Install

```bash
cd cmd/lisp
go install
```

# Execute REPL

```bash
lisp
```

Use <kbd>Ctrl</kbd> + <kbd>D</kbd> to exit Lisp REPL.

# Execute lisp program

```bash
lisp helloworld.lisp
```

# Execute inline expression

```bash
lisp -e "(+ 1 2)"
```

If both a script and `-e` are provided, the script executes first and the `-e` expression executes last. Only the `-e` result is printed.

`-e` cannot be combined with `--version` or `--test`.

A bit longer example:

```bash
lisp helloargs.lisp --eval '(do (println "evaled to" *ARGV*) 42)' ee rr
```

Will print:

```
Hello Args:  (ee rr)
evaled to (ee rr)
42
```

# Pass script arguments that look like flags

Use `--` to stop flag parsing so script arguments are passed through:

```bash
lisp -- helloworld.lisp --foo --bar
```

# Licence

This "lisp" implementation is licensed under the MPL 2.0 (Mozilla Public License 2.0). See [LICENCE](./LICENCE) for more details.
