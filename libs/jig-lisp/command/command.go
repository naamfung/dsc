package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/jig/lisp"
	"github.com/jig/lisp/repl"
	"github.com/jig/lisp/types"
)

// args represents command line arguments for the Lisp interpreter
type args struct {
	Version bool     `arg:"-v,--version" help:"show version information"`
	Test    string   `arg:"-t,--test" help:"run test suite from directory" placeholder:"DIR"`
	Debug   bool     `arg:"--debug" help:"enable DEBUG-EVAL support (may impact performance)"`
	Eval    string   `arg:"-e,--eval" help:"evaluate expression and exit" placeholder:"EXPR"`
	Script  string   `arg:"positional" help:"lisp script to execute"`
	Args    []string `arg:"positional" help:"arguments to pass to the script"`
}

func (args) Description() string {
	return "Lisp interpreter"
}

// PreParseArgs does a preliminary parse of arguments to extract the script arguments
// before libraries are loaded. This is needed because if LoadCmdLineArgs is used it
// needs to know the script arguments before the main Execute runs.
func PreParseArgs(cmdArgs []string) []string {
	var parsedArgs args
	parser, err := arg.NewParser(arg.Config{Program: "lisp"}, &parsedArgs)
	if err != nil {
		// Silently ignore parsing errors at this stage
		return []string{}
	}

	// Parse arguments (skip program name)
	if len(cmdArgs) > 1 {
		err = parser.Parse(cmdArgs[1:])
		if err != nil {
			// Silently ignore parsing errors at this stage
			return []string{}
		}
	}

	// Set scriptArgs from parsed Args (arguments after script name)
	return parsedArgs.Args
}

// Execute is the main function of a command line MAL interpreter.
// args are usually the os.Args, and repl_env contains the environment filled
// with the symbols required for the interpreter.
func Execute(cmdArgs []string, repl_env types.EnvType) error {
	var parsedArgs args
	parser, err := arg.NewParser(arg.Config{Program: "lisp"}, &parsedArgs)
	if err != nil {
		return err
	}

	// Parse arguments (skip program name)
	if len(cmdArgs) > 1 {
		err = parser.Parse(cmdArgs[1:])
		if err == arg.ErrHelp {
			parser.WriteHelp(os.Stdout)
			return nil
		}
		if err != nil {
			return err
		}
	}

	// Enable DEBUG-EVAL if flag is set
	if parsedArgs.Debug {
		lisp.DebugEvalEnabled = true
	}

	if parsedArgs.Eval != "" && (parsedArgs.Version || parsedArgs.Test != "") {
		return fmt.Errorf("-e cannot be used with --version or --test")
	}

	// Handle --version
	if parsedArgs.Version {
		versionInfo, ok := debug.ReadBuildInfo()
		if !ok {
			fmt.Println("Lisp version information unavailable")
			return nil
		}
		fmt.Printf("Lisp:\n%s\n", versionInfo)
		return nil
	}

	// Handle --test
	if parsedArgs.Test != "" {
		return runTests(parsedArgs.Test, repl_env)
	}

	// Handle file execution or stdin
	if parsedArgs.Script != "" {
		// Special case: "-" means read from stdin (like Python/Ruby)
		if parsedArgs.Script == "-" {
			// Execute from stdin (interactive mode becomes REPL)
			ctx := context.Background()
			if _, err := lisp.REPL(ctx, repl_env, `(println (str "Lisp Mal [" *host-language* "]"))`, types.NewCursorFile("REPL")); err != nil {
				return fmt.Errorf("internal error: %s", err)
			}
			if err := repl.Execute(ctx, repl_env); err != nil {
				return err
			}
		} else {
			// Execute file
			result, err := ExecuteFile(parsedArgs.Script, repl_env)
			if err != nil {
				return err
			}
			if parsedArgs.Eval == "" {
				fmt.Println(result)
			}
		}

		if parsedArgs.Eval == "" {
			return nil
		}
	}

	if parsedArgs.Eval != "" {
		ctx := context.Background()
		result, err := lisp.REPL(ctx, repl_env, parsedArgs.Eval, types.NewCursorFile("-e"))
		if err != nil {
			return err
		}
		fmt.Println(result)
		return nil
	}

	// Default: start REPL
	ctx := context.Background()
	if _, err := lisp.REPL(ctx, repl_env, `(println (str "Lisp Mal [" *host-language* "]"))`, types.NewCursorFile("REPL")); err != nil {
		return fmt.Errorf("internal error: %s", err)
	}
	return repl.Execute(ctx, repl_env)
}

// runTests executes all *_test.mal files in the given directory
func runTests(dir string, repl_env types.EnvType) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), "_test.mal") {
			testParams := fmt.Sprintf(`(def *test-params* {:test-file %q :test-absolute-path %q})`, info.Name(), path)

			ctx := context.Background()
			if _, err := lisp.REPL(ctx, repl_env, testParams, types.NewCursorFile(info.Name())); err != nil {
				return err
			}
			if _, err := lisp.REPL(ctx, repl_env, `(load-file "`+path+`")`, types.NewCursorHere(path, -3, 1)); err != nil {
				return err
			}
		}
		return nil
	})
}

// ExecuteFile executes a file on the given path
func ExecuteFile(fileName string, ns types.EnvType) (types.MalType, error) {
	ctx := context.Background()
	result, err := lisp.REPL(ctx, ns, `(load-file "`+fileName+`")`, types.NewCursorHere(fileName, -3, 1))
	if err != nil {
		return nil, err
	}
	return result, nil
}
