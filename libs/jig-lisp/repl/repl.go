package repl

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"

	goreadline "github.com/chzyer/readline"
	"github.com/jig/lisp"
	"github.com/jig/lisp/lisperror"
	"github.com/jig/lisp/types"
)

// Execute executes the main REPL loop
func Execute(ctx context.Context, repl_env types.EnvType) error {
	dirname, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	l, err := goreadline.NewEx(&goreadline.Config{
		Prompt:          "\033[32m»\033[0m ",
		HistoryFile:     dirname + "/.lisp_history",
		AutoComplete:    &lispCompleter{repl_env},
		InterruptPrompt: "^C",
		EOFPrompt:       "^D",

		HistorySearchFold:   true,
		FuncFilterInputRune: filterInput,
	})
	if err != nil {
		return err
	}
	defer l.Close()

	log.SetOutput(l.Stderr())
	var lines []string
	for {
		line, err := l.Readline()
		if err == goreadline.ErrInterrupt {
			continue
		} else if err == io.EOF {
			// proper exit with ^D
			return nil
		}

		line = strings.TrimSpace(line)
		lines = append(lines, line)
		completeLine := strings.Join(lines, "\n")

		out, err := lisp.REPL(ctx, repl_env, completeLine, types.NewCursorFile("REPL"))
		if err != nil {
			if err.Error() == "<empty line>" {
				continue
			}
			if multiLine(err) {
				l.SetPrompt("\033[31m›\033[0m ")
				continue
			}
			lines = []string{}
			l.SetPrompt("\033[32m»\033[0m ")
			switch err := err.(type) {
			case interface{ ErrorValue() types.MalType }:
				fmt.Printf("\033[31mLisp Error:\033[0m %s\n", lisp.PRINT(err.ErrorValue()))
				continue
			default:
				fmt.Printf("Error: %s\n", err)
				continue
			}
		}
		lines = []string{}
		l.SetPrompt("\033[32m»\033[0m ")
		fmt.Printf("%v\n", out)
	}
}

func multiLine(err error) bool {
	if lerr, ok := err.(lisperror.LispError); ok {
		switch typedLispError := lerr.ErrorValue().(type) {
		case error:
			switch typedLispError.Error() {
			case "expected ')', got EOF":
				return true
			case "expected ']', got EOF":
				return true
			case "expected '}', got EOF":
				return true
			case "expected '»', got EOF":
				return true
			case "expected '¬', got EOF":
				return true
			default:
				return false
			}
		default:
			return false
		}
	}
	return false
}

func filterInput(r rune) (rune, bool) {
	switch r {
	// block CtrlZ feature
	case goreadline.CharCtrlZ:
		return r, false
	}
	return r, true
}

type lispCompleter struct {
	repl_env types.EnvType
}

var re = regexp.MustCompile(`[\t\r\n \(\)\[\]\{\}]`)

func (l *lispCompleter) Do(line []rune, pos int) ([][]rune, int) {
	partial := re.Split(string(line[:pos]), -1)
	lastPartial := partial[len(partial)-1]
	var newLine [][]rune
	newLine = l.repl_env.Symbols(newLine, lastPartial)
	for _, form := range []string{
		"try",
		"finally",
		"catch",
		"fn",
		"let",
		"def",
		"defmacro",

		"do",
		"macroexpand",
		"if",
		"quote",
		"quasiquote",
		"quasiquoteexpand",
	} {
		if strings.HasPrefix(form, lastPartial) {
			newLine = append(newLine, []rune(form[len(lastPartial):]))
		}
	}
	return newLine, len(lastPartial)
}
