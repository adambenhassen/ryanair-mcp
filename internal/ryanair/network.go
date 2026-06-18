package ryanair

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const networkEndpoint = "api/views/locate/3/aggregate/all/en"

// loadNetwork fetches the network bundle, caching it for the configured TTL. It
// returns the airport list, a regular-route map, and a seasonal-route map (both
// origin IATA -> destination IATAs).
func (c *Client) loadNetwork(ctx context.Context) ([]Airport, map[string][]string, map[string][]string, error) {
	c.netMu.Lock()
	defer c.netMu.Unlock()

	if c.netCache != nil && time.Since(c.netFetched) < c.netTTL {
		return c.netCache, c.netRoutes, c.netSeasonal, nil
	}

	var resp wireNetworkResponse
	if err := getJSON(ctx, c, networkEndpoint, wwwHost+"/"+networkEndpoint, nil, &resp); err != nil {
		return nil, nil, nil, err
	}

	regionNames := namesByCode(resp.Regions)
	countryNames := namesByCode(resp.Countries)
	cityNames := namesByCode(resp.Cities)

	airports := make([]Airport, 0, len(resp.Airports))
	routes := make(map[string][]string, len(resp.Airports))
	seasonal := make(map[string][]string)
	for _, a := range resp.Airports {
		airports = append(airports, Airport{
			IataCode:     a.IataCode,
			Name:         a.Name,
			CityCode:     a.CityCode,
			CityName:     cityNames[a.CityCode],
			CountryCode:  a.CountryCode,
			CountryName:  countryNames[a.CountryCode],
			RegionCode:   a.RegionCode,
			RegionName:   regionNames[a.RegionCode],
			CurrencyCode: a.CurrencyCode,
			TimeZone:     a.TimeZone,
			Aliases:      a.Aliases,
			Latitude:     a.Coordinates.Latitude,
			Longitude:    a.Coordinates.Longitude,
			Base:         a.Base,
		})
		if r := appendRouteAirports(nil, a.Routes); len(r) > 0 {
			routes[a.IataCode] = r
		}
		if s := appendRouteAirports(nil, a.SeasonalRoutes); len(s) > 0 {
			seasonal[a.IataCode] = s
		}
	}

	c.netCache = airports
	c.netRoutes = routes
	c.netSeasonal = seasonal
	c.netFetched = time.Now()
	return airports, routes, seasonal, nil
}

// validateFilter rejects a non-empty filter value that matches no airport's
// code, turning a typo into an actionable error instead of a silently-empty
// result. An empty value (no filter) always passes.
func validateFilter(kind, value string, airports []Airport, code func(Airport) string) error {
	if value == "" {
		return nil
	}
	for _, a := range airports {
		if code(a) == value {
			return nil
		}
	}
	return fmt.Errorf("unknown %s code %q", kind, value)
}

// namesByCode indexes a list of code/name pairs by code.
func namesByCode(items []wireNamed) map[string]string {
	m := make(map[string]string, len(items))
	for _, it := range items {
		m[it.Code] = it.Name
	}
	return m
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
	airports, _, _, err := c.loadNetwork(ctx)
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
	_, routes, seasonal, err := c.loadNetwork(ctx)
	if err != nil {
		return false, err
	}
	return slices.Contains(routes[o], d) || slices.Contains(seasonal[o], d), nil
}

// ExploreDestinations lists airports reachable from an origin, flagging
// seasonal-only routes and applying optional country/region/city filters. A
// destination served by both a regular and a seasonal route is reported as
// non-seasonal. When WithFares is true, each destination carries its cheapest
// one-way fare in the given window (nil when no fare was found), via a single
// "anywhere" fares probe. When WithRouteDetails is true, each destination is
// annotated with operator/recent/tags from the searchWidget route endpoint
// (fields absent for destinations that endpoint does not report).
func (c *Client) ExploreDestinations(ctx context.Context, params ExploreParams) ([]Destination, error) {
	o := normIATA(params.Origin)
	if !validIATA(o) {
		return nil, fmt.Errorf("invalid origin IATA %q", params.Origin)
	}
	if params.WithFares && (params.Fare.DateFrom == "" || params.Fare.DateTo == "") {
		return nil, errors.New("with_fares requires date_from and date_to")
	}
	airports, routes, seasonal, err := c.loadNetwork(ctx)
	if err != nil {
		return nil, err
	}

	byCode := make(map[string]Airport, len(airports))
	for _, a := range airports {
		byCode[a.IataCode] = a
	}

	// Validate filters against the known network codes so a typo is an error,
	// not a silently-empty result (consistent with origin/group_by fail-fast).
	country := normCountry(params.Country)
	region := strings.ToUpper(strings.TrimSpace(params.Region))
	city := strings.ToUpper(strings.TrimSpace(params.City))
	if err := validateFilter("country", country, airports, func(a Airport) string { return a.CountryCode }); err != nil {
		return nil, err
	}
	if err := validateFilter("region", region, airports, func(a Airport) string { return a.RegionCode }); err != nil {
		return nil, err
	}
	if err := validateFilter("city", city, airports, func(a Airport) string { return a.CityCode }); err != nil {
		return nil, err
	}

	// Regular routes first, then seasonal-only ones, so a destination served
	// both ways is reported as non-seasonal.
	seen := make(map[string]bool)

	dests := make([]Destination, 0, len(routes[o])+len(seasonal[o]))
	add := func(code string, isSeasonal bool) {
		if seen[code] {
			return
		}
		seen[code] = true
		a, ok := byCode[code]
		if !ok {
			return
		}
		if country != "" && a.CountryCode != country {
			return
		}
		if region != "" && a.RegionCode != region {
			return
		}
		if city != "" && a.CityCode != city {
			return
		}
		dests = append(dests, Destination{Airport: a, Seasonal: isSeasonal})
	}
	for _, code := range routes[o] {
		add(code, false)
	}
	for _, code := range seasonal[o] {
		add(code, true)
	}

	if params.WithRouteDetails {
		routeMeta, err := c.AirportDestinations(ctx, o)
		if err != nil {
			return nil, err
		}
		meta := make(map[string]Destination, len(routeMeta))
		for _, r := range routeMeta {
			meta[r.IataCode] = r
		}
		for i := range dests {
			if r, ok := meta[dests[i].IataCode]; ok {
				dests[i].Operator = r.Operator
				dests[i].Recent = r.Recent
				dests[i].Tags = r.Tags
			}
		}
	}

	if !params.WithFares {
		return dests, nil
	}

	flights, err := c.OneWayFares(ctx, OneWayParams{
		Origin:   params.Origin,
		DateFrom: params.Fare.DateFrom,
		DateTo:   params.Fare.DateTo,
		Currency: params.Fare.Currency,
	})
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
