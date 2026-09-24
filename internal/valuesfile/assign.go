package valuesfile

import (
	"errors"
	"fmt"
	"strings"
)

// Assignment is one `--set component.path=value`.
type Assignment struct {
	Component string
	Path      []string
	Raw       string
}

// ParseAssignment splits `api.image.tag=v2` at the first `=` outside quotes.
// Keys are separated by dots; a dot inside a key is written `\.` as in
// `helm --set`, or the key is quoted: `api.annotations."a.b/c"=x`.
func ParseAssignment(s string) (Assignment, error) {
	keys, rest, found, err := splitKeys(s, true)
	if err != nil {
		return Assignment{}, err
	}
	if !found {
		return Assignment{}, fmt.Errorf("%q is not component.path=value", s)
	}
	if len(keys) < 2 {
		return Assignment{}, fmt.Errorf("%q names no path inside the component", s)
	}
	return Assignment{Component: keys[0], Path: keys[1:], Raw: rest}, nil
}

// ParsePath reads `component.path` for unset, where `=` has no meaning.
func ParsePath(s string) (string, []string, error) {
	keys, _, found, err := splitKeys(s, false)
	if err != nil {
		return "", nil, err
	}
	if found {
		return "", nil, fmt.Errorf("%q: unset takes a path, not a value", s)
	}
	if len(keys) < 2 {
		return "", nil, fmt.Errorf("%q names no path inside the component", s)
	}
	return keys[0], keys[1:], nil
}

func splitKeys(s string, stopAtEquals bool) (keys []string, rest string, found bool, err error) {
	var (
		cur     strings.Builder
		quoted  bool
		escaped bool
	)
	flush := func() error {
		if cur.Len() == 0 {
			return fmt.Errorf("%q has an empty key", s)
		}
		keys = append(keys, cur.String())
		cur.Reset()
		return nil
	}
	for i, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			quoted = !quoted
		case quoted:
			cur.WriteRune(r)
		case r == '.':
			if err := flush(); err != nil {
				return nil, "", false, err
			}
		case r == '=':
			if err := flush(); err != nil {
				return nil, "", false, err
			}
			if !stopAtEquals {
				return keys, "", true, nil
			}
			return keys, s[i+1:], true, nil
		default:
			cur.WriteRune(r)
		}
	}
	if quoted || escaped {
		return nil, "", false, errors.New("unterminated quote or escape in " + s)
	}
	if err := flush(); err != nil {
		return nil, "", false, err
	}
	return keys, "", false, nil
}
