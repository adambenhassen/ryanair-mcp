package tools

import (
	"context"

	"github.com/adambenhassen/ryanair-mcp/internal/ryanair"
)

// Exported for white-box testing of the pure grouping helper and the handlers.

type DestinationGroup = destinationGroup

var GroupDestinations = groupDestinations

// ExploreArgs mirrors the explore_destinations tool input for tests.
type ExploreArgs struct {
	Origin, DateFrom, DateTo, Country, Region, City, GroupBy string
	WithFares                                                bool
}

// ExploreResult mirrors exploreOutput for tests.
type ExploreResult struct {
	Destinations []ryanair.Destination
	Groups       []DestinationGroup
}

// RunExplore invokes the explore_destinations handler end-to-end.
func RunExplore(c *ryanair.Client, in ExploreArgs) (ExploreResult, error) {
	h := exploreDestinations(c)
	_, out, err := h(context.Background(), nil, exploreInput{
		Origin:    in.Origin,
		WithFares: in.WithFares,
		DateFrom:  in.DateFrom,
		DateTo:    in.DateTo,
		Country:   in.Country,
		Region:    in.Region,
		City:      in.City,
		GroupBy:   in.GroupBy,
	})
	return ExploreResult(out), err
}

// RunAnywhereUnder invokes the find_anywhere_under handler end-to-end.
func RunAnywhereUnder(c *ryanair.Client, origin, from, to string, maxPrice int) ([]ryanair.Flight, error) {
	h := findAnywhereUnder(c)
	_, out, err := h(context.Background(), nil, anywhereInput{
		Origin: origin, DateFrom: from, DateTo: to, MaxPrice: maxPrice,
	})
	return out.Flights, err
}
