package ryanair

import "time"

// --- Domain types (exported, returned to callers) ---

// Airport is a Ryanair airport with its location metadata.
type Airport struct {
	IataCode     string   `json:"iata_code"`
	Name         string   `json:"name"`
	CityName     string   `json:"city_name"`
	CityCode     string   `json:"city_code,omitempty"`
	CountryCode  string   `json:"country_code"`
	CountryName  string   `json:"country_name,omitempty"`
	RegionCode   string   `json:"region_code,omitempty"`
	RegionName   string   `json:"region_name,omitempty"`
	CurrencyCode string   `json:"currency_code,omitempty"`
	TimeZone     string   `json:"time_zone,omitempty"`
	Aliases      []string `json:"aliases,omitempty"`
	Latitude     float64  `json:"latitude,omitempty"`
	Longitude    float64  `json:"longitude,omitempty"`
	Base         bool     `json:"base,omitempty"`
}

// Flight is a single leg with its cheapest fare in the requested window.
type Flight struct {
	Origin        string     `json:"origin"`
	Destination   string     `json:"destination"`
	OriginName    string     `json:"origin_name"`
	DestName      string     `json:"destination_name"`
	DepartureTime time.Time  `json:"departure_time"`
	ArrivalTime   time.Time  `json:"arrival_time"`
	FlightNumber  string     `json:"flight_number"`
	Price         float64    `json:"price"`
	Currency      string     `json:"currency"`
	PreviousPrice *float64   `json:"previous_price,omitempty"`
	PriceUpdated  *time.Time `json:"price_updated,omitempty"`
}

// ReturnFlight pairs an outbound and inbound leg with the total trip price.
type ReturnFlight struct {
	Outbound      Flight   `json:"outbound"`
	Inbound       Flight   `json:"inbound"`
	TotalPrice    float64  `json:"total_price"`
	Currency      string   `json:"currency"`
	TripDuration  int      `json:"trip_duration_days"`
	PreviousPrice *float64 `json:"previous_price,omitempty"`
	NewRoute      bool     `json:"new_route,omitempty"`
}

// DailyFare is the cheapest fare for a single day in a price calendar.
type DailyFare struct {
	Day           string     `json:"day"`
	DepartureTime *time.Time `json:"departure_time,omitempty"`
	ArrivalTime   *time.Time `json:"arrival_time,omitempty"`
	Price         *float64   `json:"price,omitempty"`
	Currency      string     `json:"currency,omitempty"`
	SoldOut       bool       `json:"sold_out"`
	Unavailable   bool       `json:"unavailable"`
}

// TimetableFlight is one scheduled service on a route (no price).
type TimetableFlight struct {
	Day           int    `json:"day"`
	FlightNumber  string `json:"flight_number"`
	DepartureTime string `json:"departure_time"`
	ArrivalTime   string `json:"arrival_time"`
}

// Destination is a reachable airport from an origin, optionally with a fare.
type Destination struct {
	Airport

	Fare *float64 `json:"cheapest_fare,omitempty"`
}

// --- Wire types (unexported, mirror Ryanair's JSON) ---

type wirePrice struct {
	Value        float64 `json:"value"`
	CurrencyCode string  `json:"currencyCode"`
}

type wireAirport struct {
	CountryName string `json:"countryName"`
	IataCode    string `json:"iataCode"`
	Name        string `json:"name"`
	City        struct {
		Name        string `json:"name"`
		Code        string `json:"code"`
		CountryCode string `json:"countryCode"`
	} `json:"city"`
}

type wireLeg struct {
	DepartureAirport wireAirport `json:"departureAirport"`
	ArrivalAirport   wireAirport `json:"arrivalAirport"`
	DepartureDate    string      `json:"departureDate"`
	ArrivalDate      string      `json:"arrivalDate"`
	Price            wirePrice   `json:"price"`
	FlightNumber     string      `json:"flightNumber"`
	PreviousPrice    *wirePrice  `json:"previousPrice"`
	PriceUpdated     int64       `json:"priceUpdated"`
}

type wireFaresResponse struct {
	Fares []struct {
		Outbound wireLeg `json:"outbound"`
		Inbound  wireLeg `json:"inbound"`
		Summary  struct {
			Price            wirePrice  `json:"price"`
			TripDurationDays int        `json:"tripDurationDays"`
			PreviousPrice    *wirePrice `json:"previousPrice"`
			NewRoute         bool       `json:"newRoute"`
		} `json:"summary"`
	} `json:"fares"`
}

type wireDailyFare struct {
	Day           string     `json:"day"`
	DepartureDate string     `json:"departureDate"`
	ArrivalDate   string     `json:"arrivalDate"`
	Price         *wirePrice `json:"price"`
	SoldOut       bool       `json:"soldOut"`
	Unavailable   bool       `json:"unavailable"`
}

type wireCalendarResponse struct {
	Outbound struct {
		Fares []wireDailyFare `json:"fares"`
	} `json:"outbound"`
	Inbound struct {
		Fares []wireDailyFare `json:"fares"`
	} `json:"inbound"`
}

type wireScheduleResponse struct {
	Month int `json:"month"`
	Days  []struct {
		Day     int `json:"day"`
		Flights []struct {
			CarrierCode   string `json:"carrierCode"`
			Number        string `json:"number"`
			DepartureTime string `json:"departureTime"`
			ArrivalTime   string `json:"arrivalTime"`
		} `json:"flights"`
	} `json:"days"`
}

type wireNetworkAirport struct {
	IataCode     string   `json:"iataCode"`
	Name         string   `json:"name"`
	CountryCode  string   `json:"countryCode"`
	CityCode     string   `json:"cityCode"`
	RegionCode   string   `json:"regionCode"`
	CurrencyCode string   `json:"currencyCode"`
	TimeZone     string   `json:"timeZone"`
	Aliases      []string `json:"aliases"`
	Base         bool     `json:"base"`
	Coordinates  struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"coordinates"`
	Routes         []string `json:"routes"`
	SeasonalRoutes []string `json:"seasonalRoutes"`
}

type wireNamed struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type wireNetworkResponse struct {
	Airports  []wireNetworkAirport `json:"airports"`
	Regions   []wireNamed          `json:"regions"`
	Countries []wireNamed          `json:"countries"`
}
