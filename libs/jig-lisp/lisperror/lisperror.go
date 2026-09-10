package lisperror

import (
	"fmt"

	"github.com/jig/lisp/marshaler"
	"github.com/jig/lisp/printer"
	. "github.com/jig/lisp/types"
)

// StackFrame represents a single frame in the error stack trace
type StackFrame struct {
	FunctionName string    // Name of the function, macro, or special form (empty if not applicable)
	Position     *Position // Location in source code
}

// Errors/Exceptions
type LispError struct {
	err    MalType
	cursor *Position
	Stack  []*StackFrame // Stack trace of positions where error propagated
}

func (e LispError) Unwrap() error {
	if ee, ok := e.err.(error); ok {
		return ee
	}
	return nil
}

func (e LispError) ErrorValue() MalType {
	return e.err
}

func (e LispError) Is(target error) bool {
	if target == nil {
		return e.ErrorValue() == nil
	}
	// if the error (err.Err) implements its own Is() function use it
	if ee, ok := e.ErrorValue().(interface{ Is(error) bool }); ok && ee.Is(target) {
		return true
	}

	err2, ok := target.(LispError)
	if ok {
		return e.ErrorValue() == err2.ErrorValue()
	}
	return false
}

func (e LispError) Error() string {
	var msg string
	switch e.err.(type) {
	case error:
		if e.cursor != nil {
			msg = fmt.Sprintf("%s: %s", e.cursor, e.err)
		} else {
			msg = fmt.Sprint(e.err)
		}
	default:
		// TODO: this should be prt_str
		// panic("internal error: LispError.Error() called on non-error")
		if e.cursor != nil {
			msg = fmt.Sprintf("%s: %s", e.cursor, e.err)
		} else {
			msg = fmt.Sprint(e.err)
		}
	}

	// Append stack trace if available
	if len(e.Stack) > 0 {
		for _, frame := range e.Stack {
			if frame.Position != nil {
				if frame.FunctionName != "" {
					msg += fmt.Sprintf("\n  at %s (%s)", frame.FunctionName, frame.Position)
				} else {
					msg += fmt.Sprintf("\n  at %s", frame.Position)
				}
			}
		}
	}

	return msg
}

func (e LispError) Position() *Position {
	return e.cursor
}

// func (e LispError) LispPrint(_Pr_str func(obj MalType, print_readably bool) string) string {
// 	return "(error " + _Pr_str(e.err, true) + ")"
// }

// func (e LispError) Type() string {
// 	return "error"
// }

// NewGoError is used to create a LispError on errors returned by go functions
func NewGoError(fFullName string, err interface{}) error {
	switch err := err.(type) {
	case error:
		return LispError{
			err: fmt.Errorf("%s: %w", fFullName, err),
		}
	default:
		return LispError{
			err: fmt.Errorf("%s: %w", fFullName, fmt.Errorf("%v", err)),
		}
	}
}

func GetPosition(ast MalType) *Position {
	switch value := ast.(type) {
	case List:
		return value.Cursor
	case Symbol:
		return value.Cursor
	case Vector:
		return value.Cursor
	case HashMap:
		return value.Cursor
	case Set:
		return value.Cursor
	case interface{ GetPosition() *Position }:
		return value.GetPosition()
	case *Position:
		return value
	default:
		return nil
	}
}

func NewLispError(err MalType, ast MalType) LispError {
	switch err := err.(type) {
	case LispError:
		// Preserve original cursor if it exists
		if err.cursor == nil {
			err.cursor = GetPosition(ast)
		}
		// Preserve existing stack
		return err
	default:
		return LispError{
			err:    err,
			cursor: GetPosition(ast),
			Stack:  nil,
		}
	}
}

// AddStackFrame adds a position and function name to the error's stack trace
// Skips adding duplicate frames when position matches cursor and no function name is provided
func (e LispError) AddStackFrame(pos *Position, functionName string) LispError {
	if pos != nil {
		// Skip if this would duplicate the original cursor without adding useful information
		// (i.e., same position as cursor and no function name)
		if functionName == "" && e.cursor != nil && pos.String() == e.cursor.String() {
			return e
		}

		e.Stack = append(e.Stack, &StackFrame{
			FunctionName: functionName,
			Position:     pos,
		})
	}
	return e
}

func (e LispError) MarshalHashMap() (MalType, error) {
	hm := HashMap{
		Val: map[string]MalType{
			"ʞtype": fmt.Sprintf("%T", e),
		},
	}

	switch ee := e.ErrorValue().(type) {
	case marshaler.HashMap:
		pHm, err := ee.MarshalHashMap()
		if err != nil {
			return nil, err
		}
		hm.Val["ʞerr"] = pHm
	default:
		hm.Val["ʞerr"] = printer.Pr_str(ee, true)
	}

	if e.cursor != nil {
		hm.Val["ʞpos"] = e.cursor.String()
	}

	return hm, nil
}

func (e LispError) LispPrint(Pr_str func(MalType, bool) string) string {
	return "«error " + Pr_str(e.err, true) + "»"
}
