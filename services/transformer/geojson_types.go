// See GeoJSON summary format: https://earthquake.usgs.gov/earthquakes/feed/v1.0/geojson.php
package main

// we omit "tsunami" field
type Properties struct {
	Magnitude     float64  `json:"mag"`
	Place         string   `json:"place"`
	Time          int64    `json:"time"`
	Updated       int64    `json:"updated"`
	TZ            *int     `json:"tz"` // a pointer because tz can be null, means timezone is unknown
	URL           string   `json:"url"`
	Detail        string   `json:"detail"`
	Felt          int      `json:"felt"`    // DYFI system based
	CDI           float64  `json:"cdi"`     // [0.0 - 10.0] intensity computed by DYFI
	MMI           *float64 `json:"mmi"`     // [0.0 - 10.0] instrumental intensity
	Alert         *string  `json:"alert"`   // PAGER level: "green", "yellow", "orange", "red"
	Status        string   `json:"status"`  // "reviewed" means human reviewed event
	Significance  int      `json:"sig"`     // [0, 1000] event significance, represents composite value including max mmi, felt etc.
	Network       string   `json:"net"`     // id of who provided the data, "ak" = Alaska Earthquake Center
	Code          string   `json:"code"`    // unique network code of the event
	Ids           string   `json:"ids"`     // comma-separated list of event ids associated with and event "id"
	Sources       string   `json:"sources"` // comma-separated list of network contributors ",us,nc,ci"
	Types         string   `json:"types"`   // comma-separated list of event product types ",cap,dyfi,phase-data"
	NST           int      `json:"nst"`     // the total number of seismic stations used to determine the event location
	Dmin          float64  `json:"dmin"`    // [0.4, 7.1] horizontal distance (in degrees) from the epicenter to the nearest station
	RMS           float64  `json:"rms"`     // [0.13, 1.39] travel time residual in sec, allows to measure location precision, close to 0 => more precise
	Gap           int      `json:"gap"`     //[0.0, 180.0] azimuthal gap between adjacent stations (in degrees), the smaller => the more precise Dmin
	MagnitudeType string   `json:"magType"` // the algorithm used to calculate event magnitude "Md", "MI", "Ms" etc.
	Type          string   `json:"type"`    // "earthquake" or "quarry"
	Title         string   `json:"title"`   // event description e.g. "M 3.2 - 3 km ESE of San Ramon, CA"
	DminDistance  float64  // the "dmin" distance but in km, calculated based on "dmin" by the transformer
}

type Geometry struct {
	Type        string    `json:"type"` // "Point"
	Coordinates []float64 `json:"coordinates"`
}

// represents GeoJSON Feature type
type GeoFeature struct {
	Id         string     `json:"id"`
	Properties Properties `json:"properties"`
	Geometry   Geometry   `json:"geometry"`
}

type USGSFeeds struct {
	Type     string       `json:"type"`
	Features []GeoFeature `json:"features"`
}
