// See GeoJSON summary format: https://earthquake.usgs.gov/earthquakes/feed/v1.0/geojson.php
// This package contains type structs for USGS feeds json decoding.
package messages

import (
	"encoding/json"
	"time"
)

// represents GeoJSON summary feed type
type Feed string

const (
	AllHour          Feed = "all_hour"
	AllDay           Feed = "all_day"
	AllWeek          Feed = "all_week"
	AllMonth         Feed = "all_month"
	SignificantHour  Feed = "significant_hour"
	SignificantDay   Feed = "significant_day"
	SignificantWeek  Feed = "significant_week"
	SignificantMonth Feed = "significant_month"
	Mag45Hour        Feed = "4.5_hour"
	Mag25Hour        Feed = "2.5_hour"
	Mag10Hour        Feed = "1.0_hour"
	Mag45Day         Feed = "4.5_day"
	Mag25Day         Feed = "2.5_day"
	Mag10Day         Feed = "1.0_day"
	Mag45Week        Feed = "4.5_week"
	Mag25Week        Feed = "2.5_week"
	Mag10Week        Feed = "1.0_week"
	Mag45Month       Feed = "4.5_month"
	Mag25Month       Feed = "2.5_month"
	Mag10Month       Feed = "1.0_month"
)

// update this Feed map if needed, for now it contains only what we use in our manifests
var FeedTypes = map[string]Feed{
	string(AllHour):          AllHour,
	string(AllDay):           AllDay,
	string(AllWeek):          AllWeek,
	string(SignificantMonth): SignificantMonth,
}

// we omit "tsunami" field
type Properties struct {
	Magnitude     float64   `json:"mag"`
	Place         string    `json:"place"`
	Time          time.Time `json:"time"`          // rupture time in milliseconds (UTC) converted to time.Time
	Updated       time.Time `json:"updated"`       // event update time in milliseconds (UTC) converted to time.Time
	TZ            *int      `json:"tz"`            // a pointer because tz can be null, means timezone is unknown
	URL           string    `json:"url"`           // Link to USGS Event Page for event.
	Detail        string    `json:"detail"`        // Link to GeoJSON detail feed from a GeoJSON summary feed.
	Felt          int       `json:"felt"`          // DYFI system based
	CDI           float64   `json:"cdi"`           // [0.0 - 10.0] intensity computed by DYFI
	MMI           *float64  `json:"mmi"`           // [0.0 - 10.0] instrumental intensity
	Alert         *string   `json:"alert"`         // PAGER level: "green", "yellow", "orange", "red"
	Status        string    `json:"status"`        // "reviewed" means human reviewed event
	Significance  int       `json:"sig"`           // [0, 1000] event significance, represents composite value including max mmi, felt etc.
	Network       string    `json:"net"`           // id of who provided the data, "ak" = Alaska Earthquake Center
	Code          string    `json:"code"`          // unique network code of the event
	Ids           string    `json:"ids"`           // comma-separated list of event ids associated with and event "id"
	Sources       string    `json:"sources"`       // comma-separated list of network contributors ",us,nc,ci"
	Types         string    `json:"types"`         // comma-separated list of event product types ",cap,dyfi,phase-data"
	NST           int       `json:"nst"`           // the total number of seismic stations used to determine the event location
	Dmin          float64   `json:"dmin"`          // [0.4, 7.1] horizontal distance (in degrees) from the epicenter to the nearest station
	RMS           float64   `json:"rms"`           // [0.13, 1.39] travel time residual in sec, allows to measure location precision, close to 0 more precise
	Gap           int       `json:"gap"`           //[0.0, 180.0] azimuthal gap between adjacent stations (in degrees), smaller means more precise
	MagnitudeType string    `json:"magType"`       // the algorithm used to calculate event magnitude "Md", "MI", "Ms" etc.
	Type          string    `json:"type"`          // "earthquake" or "quarry"
	Title         string    `json:"title"`         // event description e.g. "M 3.2 - 3 km ESE of San Ramon, CA"
	DminDistance  float64   `json:"dmin_distance"` // the "dmin" distance but in km, calculated based on "dmin" by the transformer
}

type Geometry struct {
	Type        string    `json:"type"`        // "Point"
	Coordinates []float64 `json:"coordinates"` // longitude [-180.0, 180.0], latitude [-90.0, 90.0], earthquake depth in km [0, 1000] (extracted by the writer)
}

// represents GeoJSON Feature type
type GeoFeature struct {
	Id         string     `json:"id"`
	Properties Properties `json:"properties"`
	Geometry   Geometry   `json:"geometry"`
}

type USGSFeats struct {
	Type     string       `json:"type"`
	Features []GeoFeature `json:"features"`
}

// represents earthquake impact data which becomes available after some time only
// comes from Feature "properties" -> "products" -> "impact-text"
type ImpactData struct {
	Id   string    // Feature top level "id"
	Time time.Time // Feature top level "time"
	Text string    // "impact-text: [{"contents" : {"" : {"bytes"}}}]"
}

// HINT: We don't reuse the existing Properties because we only need to unmarshall "bytes" and few other fields.
type ImpactProperties struct {
	Time     time.Time `json:"time"`
	Products struct {
		ImpactText []struct {
			Contents map[string]struct {
				Bytes string `json:"bytes"`
			} `json:"contents"`
		} `json:"impact-text"`
	} `json:"products"`
}

type ImpactDataResponse struct {
	Properties ImpactProperties `json:"properties"`
	Id         string           `json:"id"`
}

/*
Data transformation should be done in the transformer and nowhere else, ideally.
This also includes transforming unix timestamps we fetch from USGS.
The problem is, our `Properties` struct that contains `Time` and `Updated` fields needs them to be in int64 otherwise json unmarshalling skips them.
Can we create two structs, one for unmarshaling and another for interal communication between the components? But that is duplication and more refactoring in the future.
Simpler, just add two additional fields RawTime int64, RawUpdated int64 and keep Time time.Time, Updated time.Time. Problem solved!
This pollutes our API yet a good practical solution, yes, but.
But lets use a hack because learning Go should be fun!

Let's use custom json unmarshaling with type alias, struct embedding and field shadowing, yeee boi.

This methid implements json unmarshal interface for `Properties` struct and json lib will call this method instead.
Creates anonym struct that has additional int64 Time, Updated fields that don't exist in the actual `Properties`.
The anonym struct *Dummy field is nil and needs to be initialized correctly via rewiring *Dummy pointer to point to p pointer (already initialized).
These makes dummy a fully initialized struct with shadowed Time, Updated fields which allows json unmarshaling.
*/
func (p *Properties) UnmarshalJSON(data []byte) error {
	type Dummy Properties // type aliasing to prevent json Unmarshal infinite recursion, (works because dummy won't inherit Properties method)
	dummy := struct {
		Time    int64 `json:"time"` // these fields are temp fields our json unmarshal will fill, the actual `Time time.Time` stays uninitialized
		Updated int64 `json:"updated"`
		*Dummy        // struct embedding -- Properties fields will be included into this anon struct. Well, not here, here our *Dummy field is nil
	}{Dummy: (*Dummy)(p)} // we have to initialize our nil field to already initialized p pointer, this avoids nil panic

	if err := json.Unmarshal(data, &dummy); err != nil {
		return err
	}

	// now lets update our actual Properties Time and Updated fields
	p.Time = time.UnixMilli(dummy.Time)
	p.Updated = time.UnixMilli(dummy.Updated)

	// at this point, our anon dummy with unmarshaled Time and Updated can discarded, keeping the actual p struct with the fields we need
	return nil
}

// TIP: Go generics doesn't support methods with their own type parameters so we need to duplicate UnmarshalJSON also for ImpactProperties
func (ip *ImpactProperties) UnmarshalJSON(data []byte) error {
	type Dummy ImpactProperties
	dummy := struct {
		Time int64 `json:"time"`
		*Dummy
	}{Dummy: (*Dummy)(ip)}

	if err := json.Unmarshal(data, &dummy); err != nil {
		return err
	}

	ip.Time = time.UnixMilli(dummy.Time)

	return nil

}
