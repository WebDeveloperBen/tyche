package validation

import "reflect"

type SchemaConstraints struct {
	Format    string
	MinLength *int
	MaxLength *int
	Minimum   *float64
	Maximum   *float64
	Pattern   string
	MinItems  *int
	MaxItems  *int
	Enum      []any
}

func ConstraintsForField(f reflect.StructField, schemaType string) (SchemaConstraints, error) {
	var constraints SchemaConstraints
	if format := f.Tag.Get("format"); format != "" {
		constraints.Format = format
	}
	rules, err := ParseFieldRules(f.Tag)
	if err != nil {
		return SchemaConstraints{}, err
	}
	for _, rule := range rules.Rules {
		switch rule.Kind {
		case RuleMin:
			switch schemaType {
			case "string":
				v := rule.Int
				constraints.MinLength = &v
			case "integer", "number":
				v := float64(rule.Int)
				constraints.Minimum = &v
			}
		case RuleMax:
			switch schemaType {
			case "string":
				v := rule.Int
				constraints.MaxLength = &v
			case "integer", "number":
				v := float64(rule.Int)
				constraints.Maximum = &v
			}
		case RuleLen:
			switch schemaType {
			case "string":
				min := rule.Int
				max := rule.Int
				constraints.MinLength = &min
				constraints.MaxLength = &max
			}
		case RuleMinItems:
			v := rule.Int
			constraints.MinItems = &v
		case RuleMaxItems:
			v := rule.Int
			constraints.MaxItems = &v
		case RulePattern:
			constraints.Pattern = rule.String
		case RuleOneOf:
			constraints.Enum = make([]any, len(rule.List))
			for i, p := range rule.List {
				constraints.Enum[i] = p
			}
		case RuleEmail:
			if constraints.Format == "" {
				constraints.Format = "email"
			}
		case RuleURL:
			if constraints.Format == "" {
				constraints.Format = "uri"
			}
		case RuleUUID:
			if constraints.Format == "" {
				constraints.Format = "uuid"
			}
		}
	}
	return constraints, nil
}
