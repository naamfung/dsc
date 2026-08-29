package lisp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/concurrent"
	"github.com/jig/lisp/lib/core"
	"github.com/jig/lisp/lib/coreextented"
	"github.com/jig/lisp/lib/system"
	"github.com/jig/lisp/types"
)

func TestFileTests(t *testing.T) {
	dirEntries, err := os.ReadDir("./tests")
	if err != nil {
		log.Fatal(err)
	}
	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			continue
		}
		if !strings.HasSuffix(dirEntry.Name(), ".mal") {
			continue
		}
		if !strings.HasPrefix(dirEntry.Name(), "step") {
			continue
		}
		if dirEntry.Name() == "step0_repl.mal" {
			continue
		}
		if dirEntry.Name() == "step1_read_print.mal" {
			continue
		}
		// fmt.Println(dirEntry.Name())
		code, err := os.ReadFile("./tests/" + dirEntry.Name())
		if err != nil {
			log.Fatal(err)
		}
		if err := parseFile(context.Background(), dirEntry.Name(), string(code)); err != nil {
			log.Fatal(err)
		}
	}
}

func parseFile(ctx context.Context, fileName string, code string) error {
	lines := strings.Split(string(code), "\n")
	currentLine := 0

	env := newEnv(fileName)
	var result types.MalType
	var stdoutResult string
	var lastError error
	for _, line := range lines {
		currentLine++
		line = strings.Trim(line, " \t\r\n")
		// fmt.Println(line)
		switch {
		case len(line) == 0:
			continue
		case strings.HasPrefix(line, ";;;"):
			// ignored, all tests executed
			continue
		case strings.HasPrefix(line, ";;"):
			// fmt.Println(line)
			continue
		case strings.HasPrefix(line, ";>>>"):
			// settings/commands ignored
			continue
		case strings.HasPrefix(line, ";=>"):
			line = line[3:]
			if lastError != nil {
				return fmt.Errorf("%q %000d: unexpected error: %s", fileName, currentLine, lastError)
			}
			if result != line {
				return fmt.Errorf("%q %000d: expected result `%s` got `%s`", fileName, currentLine, line, result)
			}
			lastError = nil
			continue
		case strings.HasPrefix(line, ";/"):
			line = line[2:]
			matched, err := regexp.MatchString(line, stdoutResult)
			if err != nil {
				return fmt.Errorf("%q %000d: cannot compile regex `%s` got %s", fileName, currentLine, line, err)
			}
			if !matched {
				return fmt.Errorf("%q %000d: expected stdout `%s` got `%s`", fileName, currentLine, line, stdoutResult)
			}
			lastError = nil
			continue
		case strings.HasPrefix(line, ";"):
			return fmt.Errorf("%q test data error at line %d:\n%s", fileName, currentLine, line)
		default:
			// fmt.Println(currentLine, line)
			result, stdoutResult, lastError = captureStdout(func() (types.MalType, error) {
				v, err := REPL(ctx, env, line, types.NewCursorFile(fileName))
				if v == nil {
					return "nil", err
				}
				// fmt.Fprintln(os.Stderr, "-->", result)
				return v, err
			})
			// fmt.Printf("\t\t%s\t\t\t%s\n", line, stdoutResult)
		}
	}
	return nil
}

func newEnv(fileName string) types.EnvType {
	newenv := env.NewEnv()
	core.Load(newenv)
	core.LoadInput(newenv)
	concurrent.Load(newenv)
	system.Load(newenv)
	newenv.Set(types.Symbol{Val: "eval"}, types.Func{Fn: func(ctx context.Context, a []types.MalType) (types.MalType, error) {
		return EVAL(ctx, a[0], newenv)
	}})
	newenv.Set(types.Symbol{Val: "*ARGV*"}, types.List{})

	LoadMarshalExample(newenv)
	ctx := context.Background()

	if _, err := REPL(ctx, newenv, core.HeaderBasic(), types.NewCursorFile(fileName)); err != nil {
		return nil
	}
	if _, err := REPL(ctx, newenv, core.HeaderLoadFile(), types.NewCursorFile(fileName)); err != nil {
		return nil
	}
	if _, err := REPL(ctx, newenv, coreextented.HeaderCoreExtended(), types.NewCursorFile(fileName)); err != nil {
		return nil
	}
	if _, err := REPL(ctx, newenv, concurrent.HeaderConcurrent(), types.NewCursorFile(fileName)); err != nil {
		return nil
	}

	return newenv
}

func captureStdout(REPL func() (types.MalType, error)) (result types.MalType, stdoutResult string, replError error) {
	// see https://stackoverflow.com/questions/10473800/in-go-how-do-i-capture-stdout-of-a-function-into-a-string
	// for the source example an explanation of this Go os.Pipe lines
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	os.Stdout = w

	result, errREPL := REPL()
	if errREPL != nil {
		replError = errREPL
		switch errREPL := errREPL.(type) {
		case interface{ ErrorValue() types.MalType }:
			fmt.Printf("Error: %s", PRINT(errREPL.ErrorValue()))
		default:
			fmt.Printf("Error: %s", errREPL)
		}
	}

	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		if err != nil {
			log.Println(err)
		}
		outC <- buf.String()
	}()
	w.Close()
	os.Stdout = old
	stdoutResult = <-outC
	return result, stdoutResult, replError
}
