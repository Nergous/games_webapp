// Package service defines cross-layer data contracts shared between the
// HTTP controllers and the service implementation. It intentionally has no
// interface declarations: consumers (controllers) define the interfaces
// they need in their own packages — the idiomatic Go approach — so the
// service can add methods without forcing every caller to notice.
package service

import "games_webapp/internal/models"

// ImportGameRequest is one entry in a batch import: either Name or URL must be
// non-empty (URL wins when both are provided and a slug can be extracted).
type ImportGameRequest struct {
	Name string
	URL  string
}

// ImportError describes a single failed entry in an import batch. Name echoes
// whatever the caller submitted (URL preferred, else Name) so it can be
// matched against the request.
type ImportError struct {
	Name string
	Err  string
}

// ImportResult aggregates the outcome of a batch import. Success and Errors
// are disjoint: each input ends up in exactly one of them.
type ImportResult struct {
	Success []*models.Game
	Errors  []ImportError
}
