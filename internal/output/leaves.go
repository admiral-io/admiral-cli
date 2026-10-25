package output

import (
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strconv"
)

// Leaf is one changed value at its full path, each side as JSON or Absent.
type Leaf struct{ Path, Before, After string }

// Absent is a side that has no value, shown apart from null: absent means a
// default applies, null clears it.
const Absent = "<absent>"

// Leaves expands a change into its changed leaves, so a value set inside a
// new object or list reads at its own path rather than as the whole. A side
// is JSON, or absent when its present flag is false. An object descends by
// key and a list by index; anything else is one leaf as given.
func Leaves(path string, beforePresent bool, before string, afterPresent bool, after string) []Leaf {
	bo, bIsObj := object(beforePresent, before)
	ao, aIsObj := object(afterPresent, after)
	bl, bIsList := list(beforePresent, before)
	al, aIsList := list(afterPresent, after)
	switch {
	case (bIsObj || !beforePresent) && (aIsObj || !afterPresent) && (bIsObj || aIsObj):
		return objectLeaves(path, beforePresent, before, bo, afterPresent, after, ao)
	case (bIsList || !beforePresent) && (aIsList || !afterPresent) && (bIsList || aIsList):
		return listLeaves(path, beforePresent, before, bl, afterPresent, after, al)
	}
	return []Leaf{{path, side(beforePresent, before), side(afterPresent, after)}}
}

func objectLeaves(path string, bp bool, b string, bo map[string]json.RawMessage, ap bool, a string, ao map[string]json.RawMessage) []Leaf {
	keys := slices.Collect(maps.Keys(bo))
	for k := range ao {
		if _, ok := bo[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var out []Leaf
	for _, k := range keys {
		bv, bok := bo[k]
		av, aok := ao[k]
		if bok && aok && string(bv) == string(av) {
			continue
		}
		out = append(out, Leaves(ChildPath(path, k), bok, string(bv), aok, string(av))...)
	}
	if len(out) == 0 {
		// An empty object set or cleared has no leaves to show it by.
		return []Leaf{{path, side(bp, b), side(ap, a)}}
	}
	return out
}

func listLeaves(path string, bp bool, b string, bl []json.RawMessage, ap bool, a string, al []json.RawMessage) []Leaf {
	var out []Leaf
	for i := range max(len(bl), len(al)) {
		bok, aok := i < len(bl), i < len(al)
		var bv, av string
		if bok {
			bv = string(bl[i])
		}
		if aok {
			av = string(al[i])
		}
		if bok && aok && bv == av {
			continue
		}
		out = append(out, Leaves(IndexPath(path, i), bok, bv, aok, av)...)
	}
	if len(out) == 0 {
		return []Leaf{{path, side(bp, b), side(ap, a)}}
	}
	return out
}

// object decodes js when it is present and a JSON object.
func object(present bool, js string) (map[string]json.RawMessage, bool) {
	if !present {
		return nil, false
	}
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(js), &m) != nil || m == nil {
		return nil, false
	}
	return m, true
}

// list decodes js when it is present and a non-empty JSON array; an empty
// one is shown whole.
func list(present bool, js string) ([]json.RawMessage, bool) {
	if !present {
		return nil, false
	}
	var l []json.RawMessage
	if json.Unmarshal([]byte(js), &l) != nil || len(l) == 0 {
		return nil, false
	}
	return l, true
}

// ChildPath appends a key the way a path is written on the command line:
// a key that is not a plain word is quoted.
func ChildPath(path, key string) string {
	if !plainKey.MatchString(key) {
		key = strconv.Quote(key)
	}
	if path == "" {
		return key
	}
	return path + "." + key
}

// IndexPath appends a list index: `containers[0]`.
func IndexPath(path string, i int) string {
	return path + "[" + strconv.Itoa(i) + "]"
}

var plainKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func side(present bool, js string) string {
	if !present {
		return Absent
	}
	return js
}
