package ryanair

import (
	"context"
	"fmt"
	"net/url"
)

// ActiveAirports lists every airport Ryanair currently flies, via the
// views/locate active-airports endpoint. In practice this is the same set as
// ListAirports (no filter), but served by a dedicated v5 endpoint rather than
// derived from the aggregate network bundle.
func (c *Client) ActiveAirports(ctx context.Context) ([]Airport, error) {
	const endpoint = "api/views/locate/5/airports/en/active"
	var resp []wireLocAirport
	if err := getJSON(ctx, c, endpoint, wwwHost+"/"+endpoint, nil, &resp); err != nil {
		return nil, err
	}
	airports := make([]Airport, 0, len(resp))
	for _, w := range resp {
		airports = append(airports, w.toAirport())
	}
	return airports, nil
}

// AirportInfo returns the metadata for a single airport by IATA code.
func (c *Client) AirportInfo(ctx context.Context, code string) (Airport, error) {
	a := normIATA(code)
	if !validIATA(a) {
		return Airport{}, fmt.Errorf("invalid airport IATA %q", code)
	}
	endpoint := "api/views/locate/5/airports/en/" + a
	var w wireLocAirport
	if err := getJSON(ctx, c, endpoint, wwwHost+"/"+endpoint, nil, &w); err != nil {
		return Airport{}, err
	}
	return w.toAirport(), nil
}

// AirportDestinations lists the destinations reachable from an origin via the
// searchWidget route endpoint, carrying per-destination operator, seasonal,
// recent, and tag metadata that the network bundle does not expose.
func (c *Client) AirportDestinations(ctx context.Context, origin string) ([]Destination, error) {
	o := normIATA(origin)
	if !validIATA(o) {
		return nil, fmt.Errorf("invalid origin IATA %q", origin)
	}
	endpoint := "api/views/locate/searchWidget/routes/en/airport/" + o
	var resp []wireRoute
	if err := getJSON(ctx, c, endpoint, wwwHost+"/"+endpoint, nil, &resp); err != nil {
		return nil, err
	}
	dests := make([]Destination, 0, len(resp))
	for _, r := range resp {
		dests = append(dests, Destination{
			Airport:  r.ArrivalAirport.toAirport(),
			Seasonal: r.Seasonal,
			Operator: r.Operator,
			Recent:   r.Recent,
			Tags:     r.Tags,
		})
	}
	return dests, nil
}

// NearbyAirports lists airports near the caller's IP-derived location. In a
// server context this resolves to the server's location, not the end user's.
// market is an IETF locale tag (defaults to en-gb).
func (c *Client) NearbyAirports(ctx context.Context, market string) ([]Airport, error) {
	if market == "" {
		market = "en-gb"
	}
	const endpoint = "api/geoloc/v5/nearbyAirports"
	q := url.Values{"market": {market}}
	var resp []wireLocAirport
	if err := getJSON(ctx, c, endpoint, wwwHost+"/"+endpoint, q, &resp); err != nil {
		return nil, err
	}
	airports := make([]Airport, 0, len(resp))
	for _, w := range resp {
		airports = append(airports, w.toAirport())
	}
	return airports, nil
}

// DefaultAirport returns the closest airport to the caller's IP-derived
// location. In a server context this resolves to the server's location, not
// the end user's.
func (c *Client) DefaultAirport(ctx context.Context) (Airport, error) {
	const endpoint = "api/geoloc/v5/defaultAirport"
	var w wireLocAirport
	if err := getJSON(ctx, c, endpoint, wwwHost+"/"+endpoint, nil, &w); err != nil {
		return Airport{}, err
	}
	return w.toAirport(), nil
}
