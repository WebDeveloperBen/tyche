package clientgen

import (
	"strings"
	"unicode"
)

// exportedName converts an arbitrary string (operationId, schema component key,
// field name) into a valid, exported Go identifier in PascalCase. Separators
// (-, _, /, ., space, :, {}) start new words; leading digits are prefixed.
func exportedName(s string) string {
	var b strings.Builder
	upcomingUpper := true
	for _, r := range s {
		switch {
		case r == '-' || r == '_' || r == '/' || r == '.' || r == ' ' || r == ':' || r == '{' || r == '}' || r == '[' || r == ']':
			upcomingUpper = true
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if upcomingUpper {
				b.WriteRune(unicode.ToUpper(r))
				upcomingUpper = false
			} else {
				b.WriteRune(r)
			}
		default:
			upcomingUpper = true
		}
	}
	out := b.String()
	if out == "" {
		return "X"
	}
	if r := []rune(out)[0]; unicode.IsDigit(r) {
		out = "X" + out
	}
	return fixInitialisms(out)
}

// componentBaseName recovers a readable type name from a tyche schema component
// key, which has the form "<pkgpath-with-underscores>_<TypeName>". The real Go
// type name is the segment after the final underscore.
func componentBaseName(key string) string {
	if i := strings.LastIndex(key, "_"); i >= 0 && i+1 < len(key) {
		return exportedName(key[i+1:])
	}
	return exportedName(key)
}

// commonInitialisms are upper-cased to match Go conventions (golint style).
var commonInitialisms = map[string]string{
	"Id":    "ID",
	"Url":   "URL",
	"Uri":   "URI",
	"Http":  "HTTP",
	"Https": "HTTPS",
	"Api":   "API",
	"Json":  "JSON",
	"Html":  "HTML",
	"Uuid":  "UUID",
	"Sql":   "SQL",
	"Ip":    "IP",
	"Ttl":   "TTL",
}

// fixInitialisms upper-cases a trailing/standalone common initialism so fields
// like "Id" become "ID". It only adjusts whole trailing words to stay safe.
func fixInitialisms(s string) string {
	for suffix, repl := range commonInitialisms {
		if s == suffix {
			return repl
		}
		if strings.HasSuffix(s, suffix) {
			prev := s[len(s)-len(suffix)-1]
			if prev >= 'A' && prev <= 'Z' || prev >= '0' && prev <= '9' {
				continue
			}
			return s[:len(s)-len(suffix)] + repl
		}
	}
	return s
}

// uniqueName returns name if unused in taken, otherwise appends an increasing
// numeric suffix until unique. It records the chosen name in taken.
func uniqueName(name string, taken map[string]bool) string {
	if name == "" {
		name = "T"
	}
	candidate := name
	for i := 2; taken[candidate]; i++ {
		candidate = name + itoa(i)
	}
	taken[candidate] = true
	return candidate
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
