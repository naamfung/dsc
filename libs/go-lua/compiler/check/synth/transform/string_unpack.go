package transform

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func unpackFirstValueType(formatType typ.Type) typ.Type {
	switch v := unwrap.Alias(formatType).(type) {
	case *typ.Literal:
		if v.Base != kind.String {
			return nil
		}
		format, ok := v.Value.(string)
		if !ok {
			return nil
		}
		return unpackFirstValueTypeForFormat(format)
	case *typ.Optional:
		return unpackFirstValueType(v.Inner)
	case *typ.Union:
		var members []typ.Type
		for _, member := range v.Members {
			resolved := unpackFirstValueType(member)
			if resolved == nil {
				return nil
			}
			members = append(members, resolved)
		}
		if len(members) == 0 {
			return nil
		}
		return typ.NewUnion(members...)
	case *typ.Intersection:
		format, ok := literalStringValue(v)
		if !ok {
			return nil
		}
		return unpackFirstValueTypeForFormat(format)
	default:
		return nil
	}
}

func literalStringValue(t typ.Type) (string, bool) {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Literal:
		if v.Base != kind.String {
			return "", false
		}
		s, ok := v.Value.(string)
		return s, ok
	case *typ.Intersection:
		var (
			value string
			found bool
		)
		for _, member := range v.Members {
			s, ok := literalStringValue(member)
			if !ok {
				continue
			}
			if found && s != value {
				return "", false
			}
			value = s
			found = true
		}
		return value, found
	default:
		return "", false
	}
}

func unpackFirstValueTypeForFormat(format string) typ.Type {
	for i := 0; i < len(format); {
		switch ch := format[i]; ch {
		case ' ', '\t', '\n', '\r', '<', '>', '=':
			i++
		case '!':
			i++
			start := i
			for i < len(format) && isASCIIDigit(format[i]) {
				i++
			}
			if start == i {
				return nil
			}
		case 'x':
			i++
		case 'b', 'B', 'h', 'H', 'l', 'L', 'j', 'J', 'T':
			return typ.Integer
		case 'i', 'I':
			i++
			for i < len(format) && isASCIIDigit(format[i]) {
				i++
			}
			return typ.Integer
		case 'f', 'd', 'n':
			return typ.Number
		case 'c':
			i++
			start := i
			for i < len(format) && isASCIIDigit(format[i]) {
				i++
			}
			if start == i {
				return nil
			}
			return typ.String
		case 'z', 's':
			i++
			for i < len(format) && isASCIIDigit(format[i]) {
				i++
			}
			return typ.String
		default:
			return nil
		}
	}
	return nil
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
