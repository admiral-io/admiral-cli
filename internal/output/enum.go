package output

import (
	"fmt"
	"strings"
	"unicode"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// TrimEnumPrefix returns the enum value's name with its type prefix stripped.
// Generated proto enum values are named "<TYPE>_<VALUE>" in SCREAMING_SNAKE_CASE,
// where <TYPE> is the SCREAMING_SNAKE form of the enum's CamelCase descriptor name.
// For example, RunStatus_RUN_STATUS_PLANNING returns "PLANNING".
//
// Falls back to the unmodified value name when the prefix isn't present, which
// keeps unknown values readable instead of erasing them.
func TrimEnumPrefix(e protoreflect.Enum) string {
	desc := e.Descriptor()
	value := desc.Values().ByNumber(e.Number())
	if value == nil {
		return fmt.Sprintf("%d", e.Number())
	}
	name := string(value.Name())
	prefix := camelToScreamingSnake(string(desc.Name())) + "_"
	return strings.TrimPrefix(name, prefix)
}

// FormatEnum renders an enum value as a single CamelCase token, the form the
// used for statuses and health: RUN_STATUS_PARTIALLY_FAILED
// becomes "PartiallyFailed". The zero (UNSPECIFIED) value renders as None.
func FormatEnum(e protoreflect.Enum) string {
	if e.Number() == 0 {
		return None
	}
	return screamingSnakeToCamel(TrimEnumPrefix(e))
}

// FormatEnumKebab renders an enum value in lowercase kebab-case, the form the
// used for types and kinds so output matches the words users
// type: CREDENTIAL_TYPE_SSH_KEY becomes "ssh-key", JOB_TYPE_DESTROY_PLAN
// becomes "destroy-plan". The zero (UNSPECIFIED) value renders as None.
func FormatEnumKebab(e protoreflect.Enum) string {
	if e.Number() == 0 {
		return None
	}
	return strings.ToLower(strings.ReplaceAll(TrimEnumPrefix(e), "_", "-"))
}

func screamingSnakeToCamel(s string) string {
	var b strings.Builder
	for _, w := range strings.Split(s, "_") {
		if w == "" {
			continue
		}
		b.WriteString(strings.ToUpper(w[:1]))
		b.WriteString(strings.ToLower(w[1:]))
	}
	return b.String()
}

// camelToScreamingSnake turns "ChangeSetEntryType" into "CHANGE_SET_ENTRY_TYPE".
// Treats every uppercase rune after the first as the start of a new word.
// Acronyms aren't preserved, but proto enum types in this codebase don't use them.
func camelToScreamingSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}
