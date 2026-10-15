// Package launch describes the private menu-to-game pipe protocol.
package launch

// Request starts a song using the same options as the standalone game.
// Closing the pipe shuts down the resident game, including an active song.
type Request struct {
	ID   uint64   `json:"id"`
	Args []string `json:"args"`
}

// Event reports a first draw or a saved and released song. ID identifies the
// request; ID zero is reserved for the resident process's initial idle event.
type Event struct {
	ID    uint64 `json:"id"`
	Type  string `json:"type"`
	Error string `json:"error,omitempty"`
}
