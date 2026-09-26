package note_test

import (
	"testing"
	"time"

	notev1 "github.com/unmango/slip/gen/unmango/zettelkasten/note/v1alpha1"
	"github.com/unmango/slip/internal/note"
)

func TestProto(t *testing.T) {
	now := time.Date(2026, 9, 8, 14, 12, 33, 0, time.UTC)
	n := note.New(now, "a thought\nand more")
	n.Tags = []string{"systems"}
	n.SourceURI = "https://example.com/a"

	p := n.Proto()

	if got, want := p.GetName(), "notes/202609081412"; got != want {
		t.Errorf("GetName() = %q, want %q", got, want)
	}

	if got, want := p.GetZettelId(), "202609081412"; got != want {
		t.Errorf("GetZettelId() = %q, want %q", got, want)
	}

	if got, want := p.GetTitle(), "a thought"; got != want {
		t.Errorf("GetTitle() = %q, want %q", got, want)
	}

	if got, want := p.GetBody(), "a thought\nand more"; got != want {
		t.Errorf("GetBody() = %q, want %q", got, want)
	}

	if got, want := p.GetFormat(), notev1.ContentFormat_CONTENT_FORMAT_MARKDOWN; got != want {
		t.Errorf("GetFormat() = %v, want %v", got, want)
	}

	if got, want := p.GetNoteType(), notev1.NoteType_NOTE_TYPE_FLEETING; got != want {
		t.Errorf("GetNoteType() = %v, want %v", got, want)
	}

	if got, want := p.GetSourceUri(), "https://example.com/a"; got != want {
		t.Errorf("GetSourceUri() = %q, want %q", got, want)
	}

	if !p.GetCreateTime().AsTime().Equal(now) {
		t.Errorf("GetCreateTime() = %v, want %v", p.GetCreateTime().AsTime(), now)
	}

	tags := p.GetTags()
	if len(tags) != 1 {
		t.Fatalf("GetTags() length = %d, want 1", len(tags))
	}

	if got, want := tags[0].GetName(), "systems"; got != want {
		t.Errorf("tag name = %q, want %q", got, want)
	}

	if got, want := tags[0].GetKind(), "Tag"; got != want {
		t.Errorf("tag kind = %q, want %q", got, want)
	}
}

// Output-only fields belong to whatever serves the resource, so capture must
// leave them unset rather than guessing.
func TestProtoLeavesOutputOnlyUnset(t *testing.T) {
	p := note.New(time.Date(2026, 9, 8, 14, 12, 33, 0, time.UTC), "a thought").Proto()

	if p.GetUid() != "" {
		t.Errorf("GetUid() = %q, want empty", p.GetUid())
	}

	if p.HasUpdateTime() {
		t.Error("HasUpdateTime() = true, want false")
	}

	if p.HasDeleteTime() {
		t.Error("HasDeleteTime() = true, want false")
	}

	if p.GetWordCount() != 0 {
		t.Errorf("GetWordCount() = %d, want 0", p.GetWordCount())
	}

	if p.GetBacklinkCount() != 0 {
		t.Errorf("GetBacklinkCount() = %d, want 0", p.GetBacklinkCount())
	}
}

func TestProtoOmitsEmptyOptionals(t *testing.T) {
	p := note.New(time.Date(2026, 9, 8, 14, 12, 33, 0, time.UTC), "a thought").Proto()

	if p.HasDisplayName() {
		t.Error("HasDisplayName() = true, want false")
	}

	if p.HasSourceUri() {
		t.Error("HasSourceUri() = true, want false")
	}

	if len(p.GetTags()) != 0 {
		t.Errorf("GetTags() length = %d, want 0", len(p.GetTags()))
	}
}
