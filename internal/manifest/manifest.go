// Package manifest reads admiral.yaml, the file at a repository's root that
// declares which of its directories are components. It is a list of paths
// and, where the directory's name is not a valid component name, the name to
// publish under. Nothing else: not the contract (the module is the contract),
// not versions (tags are set by whoever publishes). Publishing is by
// declaration, never by scanning a repository for things that look like
// modules.
package manifest

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Filename is the manifest's name at the repository root.
const Filename = "admiral.yaml"

// ErrNotFound is a path with no manifest at it.
var ErrNotFound = errors.New("no " + Filename + " found")

var namePattern = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ValidName reports whether s is a component name the registry accepts:
// lowercase letters, digits and hyphens, starting with a letter, at most
// 63 characters. Shared with `component publish --name`.
func ValidName(s string) bool { return namePattern.MatchString(s) }

// Component is one declared component.
type Component struct {
	// Path is the directory, relative to the repository root, forward slashes.
	Path string `yaml:"path"`
	// Name is the component name. Defaults to the directory's base name,
	// which must then be a valid name: `modules/network_hub` needs a Name.
	Name string `yaml:"name,omitempty"`
}

// Manifest is the parsed file.
type Manifest struct {
	Components []Component `yaml:"components"`
}

// Exists reports whether dir holds a manifest, which is what makes a
// directory a repository to publish rather than a component.
func Exists(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, Filename))
	return err == nil && !info.IsDir()
}

// Read loads and validates the manifest file at path. Component paths in it
// are relative to the file's directory.
func Read(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w at %s", ErrNotFound, path)
	}
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse validates the manifest's bytes: every path clean, relative and
// inside the repository, every name valid and unique.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s: %w", Filename, err)
	}
	if len(m.Components) == 0 {
		return nil, fmt.Errorf("%s: no components declared", Filename)
	}
	names := map[string]string{}
	paths := map[string]bool{}
	for i := range m.Components {
		c := &m.Components[i]
		if c.Path == "" {
			return nil, fmt.Errorf("%s: components[%d] has no path", Filename, i)
		}
		// filepath.IsAbs on the raw value catches a Windows drive path
		// (C:\modules\x), which ToSlash turns into C:/modules/x and
		// path.IsAbs would then wave through.
		clean := path.Clean(filepath.ToSlash(c.Path))
		if filepath.IsAbs(c.Path) || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("%s: path %q is not inside the repository", Filename, c.Path)
		}
		c.Path = clean
		if paths[clean] {
			return nil, fmt.Errorf("%s: path %q is declared twice", Filename, c.Path)
		}
		paths[clean] = true
		if c.Name == "" {
			c.Name = path.Base(clean)
		}
		if !ValidName(c.Name) {
			return nil, fmt.Errorf("%s: %q is not a valid component name (lowercase letters, digits and hyphens); set name: on the %q entry", Filename, c.Name, c.Path)
		}
		if other, dup := names[c.Name]; dup {
			return nil, fmt.Errorf("%s: name %q is used by both %q and %q", Filename, c.Name, other, c.Path)
		}
		names[c.Name] = c.Path
	}
	return &m, nil
}
