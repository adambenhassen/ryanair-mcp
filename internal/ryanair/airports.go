package ryanair

import (
	"context"
	"fmt"
)

// AirportInfo returns the metadata for a single airport by IATA code.
func (c *Client) AirportInfo(ctx context.Context, code string) (Airport, error) {
	a := normIATA(code)
	if !validIATA(a) {
		return Airport{}, fmt.Errorf("invalid airport IATA %q", code)
	}
	endpoint := "api/views/locate/5/airports/en/" + a
	var w wireLocAirport
	if err := getJSON(ctx, c, wwwHost, endpoint, nil, &w); err != nil {
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
	if err := getJSON(ctx, c, wwwHost, endpoint, nil, &resp); err != nil {
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
