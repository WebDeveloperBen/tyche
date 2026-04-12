package validation

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type Subject string

const (
	SubjectString     Subject = "string"
	SubjectNumber     Subject = "number"
	SubjectCollection Subject = "collection"
)

type Error struct {
	Problems []Problem `json:"errors"`
}

type Problem struct {
	Pointer string `json:"pointer"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	if e == nil || len(e.Problems) == 0 {
		return "validation failed"
	}
	if len(e.Problems) == 1 {
		return e.Problems[0].Message
	}
	return fmt.Sprintf("validation failed with %d problems", len(e.Problems))
}

func (e *Error) Empty() bool {
	return e == nil || len(e.Problems) == 0
}

func (e *Error) Add(pointer, code, message string) {
	if e == nil {
		return
	}
	e.Problems = append(e.Problems, Problem{
		Pointer: pointer,
		Code:    code,
		Message: message,
	})
}

func (e *Error) AddProblem(problem Problem) {
	if e == nil {
		return
	}
	e.Problems = append(e.Problems, problem)
}

func (e *Error) AddRequired(pointer string) {
	e.AddProblem(RequiredProblem(pointer))
}

func (e *Error) AddInvalidType(pointer string) {
	e.AddProblem(InvalidTypeProblem(pointer))
}

func (e *Error) AddRule(pointer string, kind RuleKind, subject Subject, value int) {
	e.AddProblem(RuleProblem(pointer, kind, subject, value))
}

func (e *Error) AddInvalidRule(pointer string, err error) {
	e.AddProblem(InvalidRuleProblem(pointer, err))
}

func (e *Error) Merge(other *Error) {
	if e == nil || other == nil || len(other.Problems) == 0 {
		return
	}
	e.Problems = append(e.Problems, other.Problems...)
}

func RequiredProblem(pointer string) Problem {
	return Problem{Pointer: pointer, Code: "required", Message: "Field is required."}
}

func InvalidTypeProblem(pointer string) Problem {
	return Problem{Pointer: pointer, Code: "invalid_type", Message: "Value has an invalid type or format."}
}

func InvalidRuleProblem(pointer string, err error) Problem {
	return Problem{Pointer: pointer, Code: "invalid_rule", Message: fmt.Sprintf("Validator pattern is invalid: %v.", err)}
}

func RuleProblem(pointer string, kind RuleKind, subject Subject, value int) Problem {
	switch kind {
	case RuleMin:
		switch subject {
		case SubjectString:
			return Problem{Pointer: pointer, Code: "min", Message: fmt.Sprintf("Must be at least %d characters.", value)}
		default:
			return Problem{Pointer: pointer, Code: "min", Message: fmt.Sprintf("Must be at least %d.", value)}
		}
	case RuleMax:
		switch subject {
		case SubjectString:
			return Problem{Pointer: pointer, Code: "max", Message: fmt.Sprintf("Must be at most %d characters.", value)}
		default:
			return Problem{Pointer: pointer, Code: "max", Message: fmt.Sprintf("Must be at most %d.", value)}
		}
	case RuleLen:
		switch subject {
		case SubjectString:
			return Problem{Pointer: pointer, Code: "length", Message: fmt.Sprintf("Must be exactly %d characters.", value)}
		default:
			return Problem{Pointer: pointer, Code: "length", Message: fmt.Sprintf("Must equal %d.", value)}
		}
	case RuleMinItems:
		return Problem{Pointer: pointer, Code: "min_items", Message: fmt.Sprintf("Must contain at least %d items.", value)}
	case RuleMaxItems:
		return Problem{Pointer: pointer, Code: "max_items", Message: fmt.Sprintf("Must contain at most %d items.", value)}
	case RuleOneOf:
		return Problem{Pointer: pointer, Code: "one_of", Message: "Must be one of the allowed values."}
	case RulePattern:
		return Problem{Pointer: pointer, Code: "pattern", Message: "Value is in an invalid format."}
	case RuleEmail:
		return Problem{Pointer: pointer, Code: "invalid_email", Message: "Must be a valid email address."}
	case RuleURL:
		return Problem{Pointer: pointer, Code: "invalid_url", Message: "Must be a valid URL."}
	case RuleUUID:
		return Problem{Pointer: pointer, Code: "invalid_uuid", Message: "Must be a valid UUID."}
	default:
		return Problem{Pointer: pointer, Code: string(kind), Message: "Value is invalid."}
	}
}

func JSONPointer(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		builder.WriteByte('/')
		builder.WriteString(escapeJSONPointerToken(part))
	}
	return builder.String()
}

func JoinPointer(base string, parts ...string) string {
	if base == "" {
		return JSONPointer(parts...)
	}
	var pointer strings.Builder
	pointer.WriteString(base)
	for _, part := range parts {
		if part == "" {
			continue
		}
		pointer.WriteString("/" + escapeJSONPointerToken(part))
	}
	return pointer.String()
}

func JoinPointerWithIndex(base string, index int) string {
	if base == "" {
		return strconv.Itoa(index)
	}
	return base + "/" + strconv.Itoa(index)
}

func escapeJSONPointerToken(token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	token = strings.ReplaceAll(token, "/", "~1")
	return token
}

func (p Problem) MarshalJSON() ([]byte, error) {
	type problemAlias Problem
	return json.Marshal(problemAlias(p))
}
