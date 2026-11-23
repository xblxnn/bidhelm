// Package adapters defines the boundary between SSP protocols and the auction.
package adapters

import (
	"bidhelm/internal/model"
	"net/http"
)

type Request struct {
	// PlacementID is mandatory, normalized by the adapter, independent of campaign filters.
	PlacementID string
	IP          string
	UA          string
	Context     any
}
type Adapter interface {
	Key() string
	Parse(*http.Request) (Request, error)
	Respond(http.ResponseWriter, Request, *model.Campaign) error
}
type Registry map[string]Adapter

func (r Registry) Known(k string) bool { _, ok := r[k]; return ok }
