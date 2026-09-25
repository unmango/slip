package note

import (
	refv1 "github.com/unmango/slip/gen/unmango/ref/v1alpha1"
	notev1 "github.com/unmango/slip/gen/unmango/zettelkasten/note/v1alpha1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// tagAPIVersion and tagKind address the Tag resource the proto points tags at.
const (
	tagAPIVersion = "unmango.zettelkasten.tag/v1alpha1"
	tagKind       = "Tag"
)

// Proto converts n to the wire type. The generated code is the protobuf opaque
// API, so fields are set through the builder and read through getters.
//
// Output-only fields are left unset: uid, the timestamps other than
// create_time, and every count. Those belong to whatever serves this resource.
func (n Note) Proto() *notev1.Note {
	b := notev1.Note_builder{
		Name:     proto.String(n.Name()),
		ZettelId: proto.String(n.ZettelID),
		Title:    proto.String(n.Title),
		Body:     proto.String(n.Body),
		Format:   n.Format.Enum(),
		NoteType: n.Type.Enum(),
	}

	if !n.CreateTime.IsZero() {
		b.CreateTime = timestamppb.New(n.CreateTime)
	}

	if n.DisplayName != "" {
		b.DisplayName = proto.String(n.DisplayName)
	}

	if n.SourceURI != "" {
		b.SourceUri = proto.String(n.SourceURI)
	}

	if len(n.Labels) > 0 {
		b.Labels = n.Labels
	}

	if len(n.Annotations) > 0 {
		b.Annotations = n.Annotations
	}

	for _, tag := range n.Tags {
		b.Tags = append(b.Tags, refv1.ObjectReference_builder{
			ApiVersion: proto.String(tagAPIVersion),
			Kind:       proto.String(tagKind),
			Name:       proto.String(tag),
		}.Build())
	}

	return b.Build()
}
