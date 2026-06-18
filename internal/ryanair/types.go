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

// ReturnDailyFares is the per-day cheapest fare calendar for a return trip,
// with the outbound and inbound sides side by side.
type ReturnDailyFares struct {
	Outbound []DailyFare `json:"outbound"`
	Inbound  []DailyFare `json:"inbound"`
}

// WeekendTrip is the cheapest matched Friday-departure outbound paired with its
// return inbound, with the combined price.
type WeekendTrip struct {
	Outbound   DailyFare `json:"outbound"`
	Inbound    DailyFare `json:"inbound"`
	TotalPrice float64   `json:"total_price"`
	Currency   string    `json:"currency"`
}

// TimetableFlight is one scheduled service on a route (no price).
type TimetableFlight struct {
	Day           int    `json:"day"`
	FlightNumber  string `json:"flight_number"`
	DepartureTime string `json:"departure_time"`
	ArrivalTime   string `json:"arrival_time"`
}

// Destination is a reachable airport from an origin, optionally with a fare.
// Fare is nil when fares were not requested or no fare was found. Operator,
// Recent, and Tags come from the searchWidget route endpoint — populated by
// AirportDestinations, or by ExploreDestinations when WithRouteDetails is set.
type Destination struct {
	Airport

	Seasonal bool     `json:"seasonal,omitempty"`
	Fare     *float64 `json:"cheapest_fare,omitempty"`
	Operator string   `json:"operator,omitempty"`
	Recent   bool     `json:"recent,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

// FareWindow is the date window (and optional currency) for the fares probe
// used by ExploreDestinations when WithFares is true.
type FareWindow struct {
	DateFrom string // required when WithFares is true, ISO date
	DateTo   string // required when WithFares is true, ISO date
	Currency string // optional, ISO 4217
}

// ExploreParams selects and filters reachable destinations from an origin.
type ExploreParams struct {
	Origin           string     // required, IATA
	WithFares        bool       // annotate each destination with its cheapest fare
	WithRouteDetails bool       // annotate each destination with operator/recent/tags
	Country          string     // optional ISO2 filter
	Region           string     // optional region code filter
	City             string     // optional city code filter
	Fare             FareWindow // date window for the fares probe (used when WithFares)
}

// --- Wire types (unexported, mirror Ryanair's JSON) ---

type wirePrice struct {
	Value        float64 `json:"value"`
	CurrencyCode string  `json:"currencyCode"`
}

type wireAirport struct {
	IataCode string `json:"iataCode"`
	Name     string `json:"name"`
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
	NextPage *int `json:"nextPage"`
	Size     int  `json:"size"`
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

// wireLocAirport mirrors the airport shape returned by the views/locate and
// geoloc endpoints. Region, TimeZone, Base, and Country.Currency are absent on
// the leaner geoloc responses; they map to zero values via toAirport.
type wireLocAirport struct {
	Code     string    `json:"code"`
	Name     string    `json:"name"`
	Aliases  []string  `json:"aliases"`
	Base     bool      `json:"base"`
	TimeZone string    `json:"timeZone"`
	City     wireNamed `json:"city"`
	Region   wireNamed `json:"region"`
	Country  struct {
		Code     string `json:"code"`
		Name     string `json:"name"`
		Currency string `json:"currency"`
	} `json:"country"`
	Coordinates struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"coordinates"`
}

func (w wireLocAirport) toAirport() Airport {
	return Airport{
		IataCode:     w.Code,
		Name:         w.Name,
		CityCode:     w.City.Code,
		CityName:     w.City.Name,
		CountryCode:  w.Country.Code,
		CountryName:  w.Country.Name,
		RegionCode:   w.Region.Code,
		RegionName:   w.Region.Name,
		CurrencyCode: w.Country.Currency,
		TimeZone:     w.TimeZone,
		Aliases:      w.Aliases,
		Latitude:     w.Coordinates.Latitude,
		Longitude:    w.Coordinates.Longitude,
		Base:         w.Base,
	}
}

// wireRoute mirrors one entry from the searchWidget routes endpoint: a
// reachable destination plus its operator and route metadata.
type wireRoute struct {
	ArrivalAirport wireLocAirport `json:"arrivalAirport"`
	Recent         bool           `json:"recent"`
	Seasonal       bool           `json:"seasonal"`
	Operator       string         `json:"operator"`
	Tags           []string       `json:"tags"`
}

type wireNetworkResponse struct {
	Airports  []wireNetworkAirport `json:"airports"`
	Regions   []wireNamed          `json:"regions"`
	Countries []wireNamed          `json:"countries"`
	Cities    []wireNamed          `json:"cities"`
}
