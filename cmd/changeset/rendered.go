package changeset

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/output"
)

// maxArtifactBytes caps what one artifact may unpack to, so a hostile or
// broken archive cannot fill memory.
const maxArtifactBytes = 512 << 20

// artifactFile is one regular file of a run artifact, by its slash path.
type artifactFile struct {
	name string
	data []byte
}

// readArtifact reads a run artifact's tar.gz in full. An entry that is not
// a plain file or directory, or whose path would leave the directory it is
// unpacked into, fails the whole read, so nothing is written from an
// archive that has one.
func readArtifact(gz []byte) ([]artifactFile, error) {
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, fmt.Errorf("read artifact: %w", err)
	}
	defer zr.Close() //nolint:errcheck
	tr := tar.NewReader(io.LimitReader(zr, maxArtifactBytes+1))

	var files []artifactFile
	var total int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read artifact: %w", err)
		}
		if !localPath(h.Name) {
			return nil, fmt.Errorf("artifact entry %q is outside the artifact", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
		default:
			return nil, fmt.Errorf("artifact entry %q is not a regular file", h.Name)
		}
		total += h.Size
		if total > maxArtifactBytes {
			return nil, fmt.Errorf("artifact unpacks to more than %d MiB", maxArtifactBytes>>20)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read artifact: %w", err)
		}
		files = append(files, artifactFile{name: path.Clean(h.Name), data: data})
	}
	return files, nil
}

// localPath reports whether a tar entry name stays inside the directory it
// is unpacked into: relative, no "..", no backslash a Windows path would
// read as a separator.
func localPath(name string) bool {
	if name == "" || strings.Contains(name, `\`) {
		return false
	}
	return filepath.IsLocal(filepath.FromSlash(name))
}

// renderedComponent is one component of an artifact.
type renderedComponent struct {
	name      string
	namespace string
	manifests []byte
	hooks     []byte
	findings  []artifactFinding
}

type artifactFinding struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details"`
}

// components groups an artifact's files by component, sorted by name, with
// each one's namespace from artifact.json.
func components(files []artifactFile) ([]*renderedComponent, error) {
	byName := map[string]*renderedComponent{}
	get := func(name string) *renderedComponent {
		c, ok := byName[name]
		if !ok {
			c = &renderedComponent{name: name}
			byName[name] = c
		}
		return c
	}
	var meta []byte
	for _, f := range files {
		if f.name == "artifact.json" {
			meta = f.data
			continue
		}
		parts := strings.Split(f.name, "/")
		if len(parts) != 3 || parts[0] != "components" {
			continue
		}
		c := get(parts[1])
		switch parts[2] {
		case "manifests.yaml":
			c.manifests = f.data
		case "hooks.yaml":
			c.hooks = f.data
		case "findings.json":
			if len(bytes.TrimSpace(f.data)) == 0 {
				continue
			}
			if err := json.Unmarshal(f.data, &c.findings); err != nil {
				return nil, fmt.Errorf("read artifact: %s: %w", f.name, err)
			}
		}
	}
	namespaces, err := artifactNamespaces(meta)
	if err != nil {
		return nil, err
	}
	out := make([]*renderedComponent, 0, len(byName))
	for _, c := range byName {
		c.namespace = namespaces[c.name]
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

// artifactNamespaces reads each component's namespace from artifact.json,
// whose components are a list of objects naming themselves or a map keyed
// by name.
func artifactNamespaces(meta []byte) (map[string]string, error) {
	out := map[string]string{}
	if len(bytes.TrimSpace(meta)) == 0 {
		return out, nil
	}
	var doc struct {
		Components json.RawMessage `json:"components"`
	}
	if err := json.Unmarshal(meta, &doc); err != nil {
		return nil, fmt.Errorf("read artifact: artifact.json: %w", err)
	}
	type comp struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	var list []comp
	if err := json.Unmarshal(doc.Components, &list); err == nil {
		for _, c := range list {
			out[c.Name] = c.Namespace
		}
		return out, nil
	}
	var byName map[string]comp
	if err := json.Unmarshal(doc.Components, &byName); err == nil {
		for name, c := range byName {
			out[name] = c.Namespace
		}
	}
	return out, nil
}

// writeRendered prints each component as a YAML stream: a header naming
// it, its findings as comments, its manifests, then its hooks. Secret
// values arrive masked by the server.
func writeRendered(w io.Writer, comps []*renderedComponent) {
	for i, c := range comps {
		if i > 0 {
			output.Writeln(w, "---")
		}
		ns := c.namespace
		if ns == "" {
			ns = output.None
		}
		output.Writef(w, "# component: %s  namespace: %s\n", c.name, ns)
		for _, f := range c.findings {
			output.Writef(w, "# finding: %s: %s\n", kebab(f.Code), oneLine(f.Message))
			for _, d := range f.Details {
				output.Writef(w, "#   %s\n", oneLine(d))
			}
		}
		writeYAML(w, c.manifests)
		if len(bytes.TrimSpace(c.hooks)) > 0 {
			output.Writeln(w, "---")
			output.Writeln(w, "# hooks")
			writeYAML(w, c.hooks)
		}
	}
}

func writeYAML(w io.Writer, b []byte) {
	if len(bytes.TrimSpace(b)) == 0 {
		return
	}
	_, _ = w.Write(b)
	if b[len(b)-1] != '\n' {
		output.Writeln(w)
	}
}

// kebab renders a finding code as the table does: LOOKUP_USED is
// lookup-used.
func kebab(code string) string {
	return strings.ToLower(strings.ReplaceAll(code, "_", "-"))
}

// oneLine keeps a message inside its comment.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// checkOutputDir refuses a directory with anything in it unless forced, so
// an unpack never mixes two revisions by accident.
func checkOutputDir(dir string, force bool) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 && !force {
		return cmderr.UsageHint("Pass --force to write into it anyway, or name an empty directory.",
			"%s is not empty", dir)
	}
	return nil
}

// unpack writes an artifact's files under dir. The writes go through an
// os.Root, so neither a name nor a symlink already in dir can place a file
// outside it.
func unpack(dir string, files []artifactFile) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close() //nolint:errcheck
	for _, f := range files {
		name := filepath.FromSlash(f.name)
		if d := filepath.Dir(name); d != "." {
			if err := root.MkdirAll(d, 0o755); err != nil {
				return err
			}
		}
		if err := root.WriteFile(name, f.data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
