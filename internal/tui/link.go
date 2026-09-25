package tui

// Link is a note the capture screen can link to.
type Link struct {
	ID    string
	Title string

	// Text is what gets inserted, already formatted for the notebook.
	Text string
}

// LinkSource lists the notes a capture can link to. It runs once, off the
// render loop, so the screen opens before a slow notebook finishes listing.
type LinkSource func() ([]Link, error)
