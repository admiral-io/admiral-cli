// Package valuesfile moves a component's values between the YAML a person
// edits and the JSON the API stores. `!ref users-db.host` is a reference and
// travels as {"$ref": "users-db.host"}; a real map whose only key is `$ref`
// travels as `$$ref`. Numbers keep their digits both ways.
package valuesfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const refTag = "!ref"

var (
	jsonNumber = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)
	refKey     = regexp.MustCompile(`^\$+ref$`)
)

// Parse reads a values file. Warnings name each map that had to be escaped.
func Parse(data []byte) (map[string]any, []string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, err
	}
	if doc.Kind == 0 {
		return map[string]any{}, nil, nil
	}
	var warnings []string
	v, err := fromNode(doc.Content[0], "", &warnings)
	if err != nil {
		return nil, nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, nil, errors.New("values must be a map at the top level")
	}
	return m, warnings, nil
}

// ParseScalar reads one --set value as a YAML scalar would be read: `true`
// is a bool, `3` a number, `"3"` a string, `null` a null, `!ref a.b` a
// reference.
func ParseScalar(s string) (any, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(s), &doc); err != nil {
		return nil, err
	}
	if doc.Kind == 0 {
		return "", nil
	}
	var warnings []string
	return fromNode(doc.Content[0], "", &warnings)
}

func fromNode(n *yaml.Node, at string, warnings *[]string) (any, error) {
	switch n.Kind {
	case yaml.AliasNode:
		return fromNode(n.Alias, at, warnings)
	case yaml.ScalarNode:
		return scalar(n, at)
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := fromNode(c, at, warnings)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case yaml.MappingNode:
		out := make(map[string]any, len(n.Content)/2)
		for i := 0; i < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("%s: a map key must be a string", where(at))
			}
			child := join(at, k.Value)
			if k.Tag == "!!merge" {
				return nil, fmt.Errorf("%s: merge keys (<<) are not supported", where(child))
			}
			v, err := fromNode(n.Content[i+1], child, warnings)
			if err != nil {
				return nil, err
			}
			out[k.Value] = v
		}
		if len(out) == 1 {
			for k, v := range out {
				if refKey.MatchString(k) {
					*warnings = append(*warnings, fmt.Sprintf("%s: a map whose only key is %s is stored as $%s; write !ref to reference an output", where(at), k, k))
					return map[string]any{"$" + k: v}, nil
				}
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s: unsupported YAML node", where(at))
	}
}

func scalar(n *yaml.Node, at string) (any, error) {
	if n.Tag == refTag {
		return map[string]any{"$ref": n.Value}, nil
	}
	switch n.ShortTag() {
	case "!!null":
		return nil, nil
	case "!!bool":
		var b bool
		if err := n.Decode(&b); err != nil {
			return nil, fmt.Errorf("%s: %w", where(at), err)
		}
		return b, nil
	case "!!int", "!!float":
		if jsonNumber.MatchString(n.Value) {
			return json.Number(n.Value), nil
		}
		var v any
		if err := n.Decode(&v); err != nil {
			return nil, fmt.Errorf("%s: %w", where(at), err)
		}
		switch t := v.(type) {
		case int:
			return json.Number(strconv.Itoa(t)), nil
		case int64:
			return json.Number(strconv.FormatInt(t, 10)), nil
		case uint64:
			return json.Number(strconv.FormatUint(t, 10)), nil
		case float64:
			if math.IsNaN(t) || math.IsInf(t, 0) {
				return nil, fmt.Errorf("%s: %s is not a number JSON can hold", where(at), n.Value)
			}
			return json.Number(strconv.FormatFloat(t, 'g', -1, 64)), nil
		}
		return nil, fmt.Errorf("%s: %s is not a number", where(at), n.Value)
	case "!!str", "!!timestamp", "!!binary":
		return n.Value, nil
	default:
		return nil, fmt.Errorf("%s: tag %s is not supported (only !ref is)", where(at), n.Tag)
	}
}

// Header is what a downloaded file says about where it came from; upload
// sends Revision back so a stale file is refused.
type Header struct {
	ChangeSet  string
	Component  string
	Revision   int32
	BaseDigest string
}

var revisionLine = regexp.MustCompile(`^#\s*revision:\s*([0-9]+)\s*$`)

// ParseHeader reads the revision from the leading comment lines.
func ParseHeader(data []byte) (Header, bool) {
	var h Header
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") {
			break
		}
		if m := revisionLine.FindStringSubmatch(line); m != nil {
			n, err := strconv.ParseInt(m[1], 10, 32)
			if err != nil {
				return h, false
			}
			h.Revision = int32(n)
			return h, true
		}
	}
	return h, false
}

// Render writes a values file: the header, then the tree as YAML with keys
// sorted, references as !ref, and escaped maps unescaped.
func Render(tree map[string]any, h Header) ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# change set: %s\n# component: %s\n# revision: %d\n", h.ChangeSet, h.Component, h.Revision)
	if h.BaseDigest != "" {
		fmt.Fprintf(&b, "# base: %s\n", h.BaseDigest)
	}
	b.WriteString("# Edit and upload with: admiral cs values " + h.ChangeSet + " " + h.Component + " --values <file>\n")
	if len(tree) == 0 {
		b.WriteString("{}\n")
		return b.Bytes(), nil
	}
	node, err := toNode(tree)
	if err != nil {
		return nil, err
	}
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func toNode(v any) (*yaml.Node, error) {
	switch t := v.(type) {
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(t)}, nil
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: t}, nil
	case json.Number:
		tag := "!!float"
		if _, err := strconv.ParseInt(string(t), 10, 64); err == nil || !strings.ContainsAny(string(t), ".eE") {
			tag = "!!int"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: string(t)}, nil
	case []any:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, e := range t {
			c, err := toNode(e)
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, c)
		}
		return n, nil
	case map[string]any:
		if len(t) == 1 {
			if target, ok := t["$ref"].(string); ok {
				return &yaml.Node{Kind: yaml.ScalarNode, Tag: refTag, Value: target}, nil
			}
			for k, e := range t {
				if strings.HasPrefix(k, "$$") && refKey.MatchString(k) {
					t = map[string]any{k[1:]: e}
				}
			}
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, k := range keys {
			c, err := toNode(t[k])
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}, c)
		}
		return n, nil
	default:
		return nil, fmt.Errorf("values: unsupported type %T", v)
	}
}

// DecodeJSON reads the API's JSON with numbers kept exact.
func DecodeJSON(s string) (map[string]any, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// EncodeJSON writes a value for the API. encoding/json writes a json.Number
// verbatim, so digits survive.
func EncodeJSON(v any) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

func join(at, key string) string {
	if at == "" {
		return key
	}
	return at + "." + key
}

func where(at string) string {
	if at == "" {
		return "values"
	}
	return at
}
