package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

type weatherLocation struct {
	Name      string  `json:"name"`
	Region    string  `json:"admin1"`
	Country   string  `json:"country"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func (place weatherLocation) label() string {
	name := place.Name
	if place.Region != "" && place.Region != place.Name {
		name += ", " + place.Region
	}
	if place.Country != "" {
		name += ", " + place.Country
	}
	return name
}

func validLocationQuery(query string) bool {
	return utf8.RuneCountInString(strings.TrimSpace(query)) >= 2 && len(query) <= 120
}

func searchWeatherLocations(ctx context.Context, query string) ([]weatherLocation, error) {
	if !validLocationQuery(query) {
		return nil, errors.New("enter a city or town name (2 to 120 characters)")
	}
	args := url.Values{"name": {strings.TrimSpace(query)}, "count": {"8"}, "language": {"en"}}
	request, err := http.NewRequestWithContext(ctx, "GET", "https://geocoding-api.open-meteo.com/v1/search?"+args.Encode(), nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 6 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("could not reach Open-Meteo location search")
	}
	defer response.Body.Close()
	var result struct {
		Results []weatherLocation `json:"results"`
	}
	if response.StatusCode != 200 || json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&result) != nil {
		return nil, errors.New("Open-Meteo location search is unavailable")
	}
	places := make([]weatherLocation, 0, 8)
	for _, place := range result.Results {
		if place.Name != "" && len(place.label()) <= 240 && place.Latitude >= -90 && place.Latitude <= 90 && place.Longitude >= -180 && place.Longitude <= 180 {
			places = append(places, place)
			if len(places) == cap(places) {
				break
			}
		}
	}
	return places, nil
}
