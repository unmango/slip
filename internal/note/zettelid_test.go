package note_test

import (
	"testing"
	"time"

	"github.com/unmango/slip/internal/note"
)

func TestZettelID(t *testing.T) {
	// A zone with a non-zero offset, so a UTC-vs-local mistake is visible.
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("loading location: %v", err)
	}

	tests := map[string]struct {
		in   time.Time
		want string
	}{
		"afternoon":       {time.Date(2026, 9, 8, 14, 12, 33, 0, chicago), "202609081412"},
		"midnight":        {time.Date(2026, 9, 8, 0, 0, 0, 0, chicago), "202609080000"},
		"single digits":   {time.Date(2026, 1, 2, 3, 4, 0, 0, chicago), "202601020304"},
		"end of year":     {time.Date(2026, 12, 31, 23, 59, 59, 0, chicago), "202612312359"},
		"dst spring fwd":  {time.Date(2026, 3, 8, 3, 30, 0, 0, chicago), "202603080330"},
		"sub-minute lost": {time.Date(2026, 9, 8, 14, 12, 59, 999, chicago), "202609081412"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := note.ZettelID(tt.in); got != tt.want {
				t.Errorf("ZettelID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestZettelIDIsLocal(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("loading location: %v", err)
	}

	// 2026-09-08T23:00 in Tokyo is the 8th there and the 8th at 14:00 UTC. The
	// ID must read off the clock it was given, not UTC.
	got := note.ZettelID(time.Date(2026, 9, 8, 23, 0, 0, 0, tokyo))
	if want := "202609082300"; got != want {
		t.Errorf("ZettelID() = %q, want %q", got, want)
	}
}

func TestNextID(t *testing.T) {
	tests := map[string]struct {
		taken []string
		want  string
	}{
		"free":         {nil, "202609081412"},
		"one taken":    {[]string{"202609081412"}, "202609081412a"},
		"two taken":    {[]string{"202609081412", "202609081412a"}, "202609081412b"},
		"through z":    {throughZ("202609081412"), "202609081412aa"},
		"past aa":      {append(throughZ("202609081412"), "202609081412aa"), "202609081412ab"},
		"unrelated id": {[]string{"202609081413"}, "202609081412"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			taken := make(map[string]bool, len(tt.taken))
			for _, id := range tt.taken {
				taken[id] = true
			}

			got := note.NextID("202609081412", func(id string) bool { return taken[id] })
			if got != tt.want {
				t.Errorf("NextID() = %q, want %q", got, tt.want)
			}
		})
	}
}

// throughZ lists id plus every single-letter suffix, exhausting the one-char
// ladder so the next free id has to roll over to two.
func throughZ(id string) []string {
	out := []string{id}
	for c := byte('a'); c <= 'z'; c++ {
		out = append(out, id+string(c))
	}

	return out
}
