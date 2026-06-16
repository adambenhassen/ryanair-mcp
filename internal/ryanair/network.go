package ryanair

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

const networkEndpoint = "api/views/locate/3/aggregate/all/en"

// loadNetwork fetches the network bundle, caching it for the configured TTL.
// It returns the airport list and a route map (origin IATA -> destination IATAs).
func (c *Client) loadNetwork(ctx context.Context) ([]Airport, map[string][]string, error) {
	c.netMu.Lock()
	defer c.netMu.Unlock()

	if c.netCache != nil && time.Since(c.netFetched) < c.netTTL {
		return c.netCache, c.netRoutes, nil
	}

	var resp wireNetworkResponse
	if err := getJSON(ctx, c, networkEndpoint, wwwHost+"/"+networkEndpoint, nil, &resp); err != nil {
		return nil, nil, err
	}

	airports := make([]Airport, 0, len(resp.Airports))
	routes := make(map[string][]string, len(resp.Airports))
	for _, a := range resp.Airports {
		airports = append(airports, Airport{
			IataCode:    a.IataCode,
			Name:        a.Name,
			CountryCode: a.CountryCode,
			Latitude:    a.Coordinates.Latitude,
			Longitude:   a.Coordinates.Longitude,
			Base:        a.Base,
		})
		dests := make([]string, 0, len(a.Routes)+len(a.SeasonalRoutes))
		dests = appendRouteAirports(dests, a.Routes)
		dests = appendRouteAirports(dests, a.SeasonalRoutes)
		if len(dests) > 0 {
			routes[a.IataCode] = dests
		}
	}

	c.netCache = airports
	c.netRoutes = routes
	c.netFetched = time.Now()
	return airports, routes, nil
}

// appendRouteAirports extracts destination IATAs from "airport:XXX" route
// strings, ignoring city/country/region entries.
func appendRouteAirports(dst, routes []string) []string {
	for _, r := range routes {
		if code, ok := strings.CutPrefix(r, "airport:"); ok {
			dst = append(dst, code)
		}
	}
	return dst
}

// ListAirports returns all airports, optionally filtered by ISO2 country code.
func (c *Client) ListAirports(ctx context.Context, country string) ([]Airport, error) {
	airports, _, err := c.loadNetwork(ctx)
	if err != nil {
		return nil, err
	}
	country = normCountry(country)
	if country == "" {
		return airports, nil
	}
	filtered := make([]Airport, 0, len(airports))
	for _, a := range airports {
		if a.CountryCode == country {
			filtered = append(filtered, a)
		}
	}
	return filtered, nil
}

// ValidateRoute reports whether origin has a (scheduled or seasonal) route to
// dest in Ryanair's network.
func (c *Client) ValidateRoute(ctx context.Context, origin, dest string) (bool, error) {
	o, d := normIATA(origin), normIATA(dest)
	if !validIATA(o) || !validIATA(d) {
		return false, fmt.Errorf("invalid route %q-%q", origin, dest)
	}
	_, routes, err := c.loadNetwork(ctx)
	if err != nil {
		return false, err
	}
	return slices.Contains(routes[o], d), nil
}

// ExploreDestinations lists airports reachable from origin. When withFares is
// true, each destination carries its cheapest one-way fare in the given window
// (nil when no fare was found), via a single "anywhere" fares probe.
func (c *Client) ExploreDestinations(ctx context.Context, origin string, withFares bool, fare OneWayParams) ([]Destination, error) {
	o := normIATA(origin)
	if !validIATA(o) {
		return nil, fmt.Errorf("invalid origin IATA %q", origin)
	}
	airports, routes, err := c.loadNetwork(ctx)
	if err != nil {
		return nil, err
	}

	byCode := make(map[string]Airport, len(airports))
	for _, a := range airports {
		byCode[a.IataCode] = a
	}

	dests := make([]Destination, 0, len(routes[o]))
	for _, code := range routes[o] {
		if a, ok := byCode[code]; ok {
			dests = append(dests, Destination{Airport: a})
		}
	}

	if !withFares {
		return dests, nil
	}

	fare.Origin = origin
	flights, err := c.OneWayFares(ctx, fare)
	if err != nil {
		return nil, err
	}
	cheapest := make(map[string]float64, len(flights))
	for _, f := range flights {
		if cur, ok := cheapest[f.Destination]; !ok || f.Price < cur {
			cheapest[f.Destination] = f.Price
		}
	}
	for i := range dests {
		if price, ok := cheapest[dests[i].IataCode]; ok {
			p := price
			dests[i].Fare = &p
		}
	}
	return dests, nil
}
