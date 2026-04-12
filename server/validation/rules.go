package validation

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type RuleKind string

const (
	RuleMin       RuleKind = "min"
	RuleMax       RuleKind = "max"
	RuleLen       RuleKind = "len"
	RuleMinItems  RuleKind = "minItems"
	RuleMaxItems  RuleKind = "maxItems"
	RuleOneOf     RuleKind = "oneof"
	RulePattern   RuleKind = "pattern"
	RuleEmail     RuleKind = "email"
	RuleURL       RuleKind = "url"
	RuleUUID      RuleKind = "uuid"
	RuleRequired  RuleKind = "required"
	RuleOmitEmpty RuleKind = "omitempty"
)

type Rule struct {
	Kind   RuleKind
	Int    int
	String string
	List   []string
}

type FieldRules struct {
	Required  bool
	OmitEmpty bool
	Rules     []Rule
	ItemRules []Rule
}

func ParseFieldRules(tag reflect.StructTag) (FieldRules, error) {
	var out FieldRules

	if raw := tag.Get("validate"); raw != "" {
		parts := strings.SplitSeq(raw, ",")
		for part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			switch part {
			case string(RuleRequired):
				out.Required = true
			case string(RuleOmitEmpty):
				out.OmitEmpty = true
			case string(RuleEmail):
				out.Rules = append(out.Rules, Rule{Kind: RuleEmail})
			case string(RuleURL):
				out.Rules = append(out.Rules, Rule{Kind: RuleURL})
			case string(RuleUUID):
				out.Rules = append(out.Rules, Rule{Kind: RuleUUID})
			default:
				key, value, ok := strings.Cut(part, "=")
				if after, ok0 := strings.CutPrefix(part, "items."); ok0 {
					itemRule, err := parseItemsRule(after)
					if err != nil {
						return FieldRules{}, err
					}
					out.ItemRules = append(out.ItemRules, itemRule)
					continue
				}
				if !ok {
					return FieldRules{}, fmt.Errorf("unsupported validation rule %q", part)
				}
				rule, err := parseKeyValueRule(strings.TrimSpace(key), strings.TrimSpace(value))
				if err != nil {
					return FieldRules{}, err
				}
				out.Rules = append(out.Rules, rule)
			}
		}
	}

	if out.Required && out.OmitEmpty {
		return FieldRules{}, fmt.Errorf("validation rules cannot contain both %q and %q", RuleRequired, RuleOmitEmpty)
	}

	return out, nil
}

func parseKeyValueRule(key, value string) (Rule, error) {
	switch RuleKind(key) {
	case RuleMin, RuleMax, RuleLen, RuleMinItems, RuleMaxItems:
		n, err := strconv.Atoi(value)
		if err != nil {
			return Rule{}, fmt.Errorf("invalid %s value %q", key, value)
		}
		return Rule{Kind: RuleKind(key), Int: n}, nil
	case RulePattern:
		if value == "" {
			return Rule{}, fmt.Errorf("pattern validation requires a value")
		}
		return Rule{Kind: RulePattern, String: value}, nil
	case RuleOneOf:
		values := strings.Fields(value)
		if len(values) == 0 {
			return Rule{}, fmt.Errorf("oneof validation requires at least one value")
		}
		return Rule{Kind: RuleOneOf, List: values}, nil
	default:
		return Rule{}, fmt.Errorf("unsupported validation rule %q", key)
	}
}

func parseItemsRule(raw string) (Rule, error) {
	switch raw {
	case string(RuleEmail):
		return Rule{Kind: RuleEmail}, nil
	case string(RuleURL):
		return Rule{Kind: RuleURL}, nil
	case string(RuleUUID):
		return Rule{Kind: RuleUUID}, nil
	}
	key, value, ok := strings.Cut(raw, "=")
	if !ok {
		return Rule{}, fmt.Errorf("unsupported validation rule %q", "items."+raw)
	}
	rule, err := parseKeyValueRule(strings.TrimSpace(key), strings.TrimSpace(value))
	if err != nil {
		return Rule{}, err
	}
	return rule, nil
}
