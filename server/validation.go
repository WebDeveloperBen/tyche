package server

import "github.com/webdeveloperben/tyche/server/validation"

type ValidationError = validation.Error
type ValidationProblem = validation.Problem
type ValidationSubject = validation.Subject

const (
	ValidationSubjectString     = validation.SubjectString
	ValidationSubjectNumber     = validation.SubjectNumber
	ValidationSubjectCollection = validation.SubjectCollection
)

const (
	ValidationRuleMin      = validation.RuleMin
	ValidationRuleMax      = validation.RuleMax
	ValidationRuleLen      = validation.RuleLen
	ValidationRuleMinItems = validation.RuleMinItems
	ValidationRuleMaxItems = validation.RuleMaxItems
	ValidationRuleOneOf    = validation.RuleOneOf
	ValidationRulePattern  = validation.RulePattern
	ValidationRuleEmail    = validation.RuleEmail
	ValidationRuleURL      = validation.RuleURL
	ValidationRuleUUID     = validation.RuleUUID
)

func ValidateUUID(value string) bool {
	return validation.ValidateUUID(value)
}

func ValidationStringLength(value string) int {
	return validation.StringLength(value)
}

func JoinValidationPointer(base string, parts ...string) string {
	return validation.JoinPointer(base, parts...)
}
