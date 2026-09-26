package note

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	notev1 "github.com/unmango/slip/gen/unmango/zettelkasten/note/v1alpha1"
	"gopkg.in/yaml.v3"
)

// fence delimits the frontmatter block. zk recognises the same delimiter, so a
// note slip writes is indexed without any conversion step.
const fence = "---\n"

// frontmatter is the on-disk metadata. Keys are the proto field names verbatim
// and enums are their full proto names, which keeps the mapping to Note
// mechanical and the file close to protojson.
//
// Field order here is the emitted order: yaml.v3 marshals a struct in
// declaration order.
type frontmatter struct {
	ZettelID    string            `yaml:"zettel_id"`
	Title       string            `yaml:"title"`
	NoteType    string            `yaml:"note_type"`
	Format      string            `yaml:"format"`
	CreateTime  time.Time         `yaml:"create_time"`
	Tags        []string          `yaml:"tags,omitempty"`
	SourceURI   string            `yaml:"source_uri,omitempty"`
	DisplayName string            `yaml:"display_name,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

// Marshal renders n as a markdown file: a frontmatter block, a blank line, then
// the body exactly as it was captured.
func Marshal(n Note) ([]byte, error) {
	meta, err := yaml.Marshal(frontmatter{
		ZettelID:    n.ZettelID,
		Title:       n.Title,
		NoteType:    n.Type.String(),
		Format:      n.Format.String(),
		CreateTime:  n.CreateTime,
		Tags:        n.Tags,
		SourceURI:   n.SourceURI,
		DisplayName: n.DisplayName,
		Labels:      n.Labels,
		Annotations: n.Annotations,
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling frontmatter: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString(fence)
	buf.Write(meta)
	buf.WriteString(fence)
	buf.WriteString("\n")
	buf.WriteString(n.Body)

	return buf.Bytes(), nil
}

// Unmarshal parses a markdown file written by Marshal.
func Unmarshal(b []byte) (Note, error) {
	rest, ok := strings.CutPrefix(string(b), fence)
	if !ok {
		return Note{}, fmt.Errorf("no frontmatter: file does not start with %q", strings.TrimSpace(fence))
	}

	meta, body, ok := strings.Cut(rest, "\n"+fence)
	if !ok {
		return Note{}, fmt.Errorf("no frontmatter: unterminated %q block", strings.TrimSpace(fence))
	}

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(meta), &fm); err != nil {
		return Note{}, fmt.Errorf("parsing frontmatter: %w", err)
	}

	noteType, err := parseEnum(notev1.NoteType_value, fm.NoteType, "note_type")
	if err != nil {
		return Note{}, err
	}

	format, err := parseEnum(notev1.ContentFormat_value, fm.Format, "format")
	if err != nil {
		return Note{}, err
	}

	return Note{
		ZettelID:    fm.ZettelID,
		Title:       fm.Title,
		Body:        strings.TrimPrefix(body, "\n"),
		Type:        notev1.NoteType(noteType),
		Format:      notev1.ContentFormat(format),
		Tags:        fm.Tags,
		SourceURI:   fm.SourceURI,
		DisplayName: fm.DisplayName,
		Labels:      fm.Labels,
		Annotations: fm.Annotations,
		CreateTime:  fm.CreateTime,
	}, nil
}

// parseEnum resolves a full proto enum name against the generated value map. An
// absent key is the zero value rather than an error, so a hand-written note may
// omit it.
func parseEnum(values map[string]int32, name, field string) (int32, error) {
	if name == "" {
		return 0, nil
	}

	v, ok := values[name]
	if !ok {
		return 0, fmt.Errorf("unknown %s %q", field, name)
	}

	return v, nil
}
