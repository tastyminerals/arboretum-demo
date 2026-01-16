// This package implements a service that queries USGS API for up-to-date earthquake data.
// It is has a boring name: "fetcher".
// The fetcher spawns a goroute to fetch + push JSON data to our message broker.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tastyminerals/arboretum-demo/services/internal/messages"
	"github.com/tastyminerals/arboretum-demo/services/internal/natsutils"
)

var feedsUrl = "https://earthquake.usgs.gov/earthquakes/feed/v1.0/summary/"
var eventsUrl = "https://earthquake.usgs.gov/fdsnws/event/1/query?format=geojson&producttype=impact-text&starttime=%s&endtime=%s&minsig=600"

// Get event detail response, unmarshal it into ImpactDataResponse and return ImpactData if possible.
func buildImpactData(ctx context.Context, detailUrl string) (messages.ImpactData, error) {
	data, err := httpGet(ctx, detailUrl, 30)
	if err != nil {
		return messages.ImpactData{}, err
	}

	var idr messages.ImpactDataResponse
	if err := json.Unmarshal(data, &idr); err != nil {
		return messages.ImpactData{}, fmt.Errorf("unmarshaling impact data response failed: %w", err)
	}

	if len(idr.Properties.Products.ImpactText) == 0 {
		return messages.ImpactData{}, fmt.Errorf("impact-text data doesn't exist")
	}
	return messages.ImpactData{Id: idr.Id, Time: idr.Properties.Time, Text: idr.Properties.Products.ImpactText[0].Contents[""].Bytes}, nil

}

func fetchImpactData(ctx context.Context) ([][]byte, error) {
	// Get last 30 days events from USGS API
	// TIP: use AddDate instead of plain time.Date arithmetic which can trigger Go normalization if Feb 31 -> March 2d or 3d
	url := fmt.Sprintf(eventsUrl, time.Now().AddDate(0, 0, -30).Format("2006-01-02"), time.Now().Format("2006-01-02"))
	data, err := httpGet(ctx, url, 60)
	if err != nil {
		return nil, err
	}

	var usgsFeats messages.USGSFeats
	if err := json.Unmarshal(data, &usgsFeats); err != nil {
		return nil, fmt.Errorf("unmarshaling events data failed: %w", err)
	}

	// For each earthquake event retrieve the impact-text data if available, unmarshal -> ImpactData -> marshal into [][]byte.
	var bbytes [][]byte
	for _, feat := range usgsFeats.Features {
		impData, err := buildImpactData(ctx, feat.Properties.Detail)
		if err != nil {
			log.Printf("failed to build impact data due to %v from %s", err, feat.Properties.Detail)
			continue
		}
		data, err := json.Marshal(impData)
		if err != nil {
			log.Printf("failed to marshal impact data due to %v", err)
			continue
		}
		bbytes = append(bbytes, data)
	}

	return bbytes, nil
}

/*
Create http Client and perform a series of GET requests to fetch USGS impact data from GeoJSON summary and detail.
The function performs the following requests:

 1. First GET retrieves a list of the most significant events that contains "impact-text" data from GeoJSON summary feeds.
    The "impact-data" can be retrieved using "detail" event url.
 2. Then, for each event, the function performs a GET request to retrieve the data from GeoJSON detail that contain "impact-text".
 3. We parse the response to extract "impact-text" as well as "event_id", "event_time", "updated" values.
 4. Finally, we marshal and publish to NATS.

https://earthquake.usgs.gov/fdsnws/event/1/query?format=geojson&producttype=impact-text&starttime=2025-01-01&endtime=2026-01-15&minsig=600
https://earthquake.usgs.gov/fdsnws/event/1/query?eventid=us6000qw60&format=geojson
*/
func fetchImpactDataAndPublish(ctx context.Context, natsUrl string, subject string, pollFreq time.Duration, feedType messages.Feed) {
	// create unauthenticated NATS connection, NATS doesn't require unique names but it makes debugging easier
	nc, err := natsutils.Connect(natsUrl, "arboretum-fetcher-"+string(feedType))
	if err != nil {
		log.Fatalf("failed to connect to NATS due to %v\n", err)
	}

	defer func() {
		if err := nc.Drain(); err != nil {
			log.Printf("failed to drain NATS connection: %v", err)
		}
	}()

	withPolling(ctx, fetchImpactData, nc, subject, pollFreq)

}

// Create http client and perform a GET request to fetch USGS feed data.
func fetchFeeds(ctx context.Context, feedType messages.Feed) ([][]byte, error) {
	data, err := httpGet(ctx, feedsUrl+string(feedType)+".geojson", 30)
	if err != nil {
		return nil, err
	}
	return [][]byte{data}, nil

}

// This function fetches the USGS feed as JSON once per poll frequency set in the manifest and pushes it to the message broker.
// The function performs up to 5 retries with simple backoff.
// HINT: The update frequency is set to once per minute because this is the frequency of USGS feeds update.
func fetchFeedsAndPublish(ctx context.Context, url string, subject string, pollFreq time.Duration, feedType messages.Feed) {

	// create unauthenticated NATS connection, NATS doesn't require unique names but it makes debugging easier
	nc, err := natsutils.Connect(url, "arboretum-fetcher-"+string(feedType))
	if err != nil {
		log.Fatalf("failed to connect to NATS due to %v\n", err)
	}

	defer func() {
		if err := nc.Drain(); err != nil {
			log.Printf("failed to drain NATS connection: %v", err)
		}
	}()

	// HINT: lets practice Go closures here
	withPolling(ctx, func(ctx context.Context) ([][]byte, error) {
		return fetchFeeds(ctx, feedType)
	}, nc, subject, pollFreq)
}

func main() {
	// create an empty root context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // if main exists or panics, this will be called

	url := os.Getenv("NATS_URL")
	if url == "" {
		url = "nats://nats:4222"
	}

	pf := os.Getenv("POLL_FREQUENCY")
	if pf == "" {
		log.Println("POLL_FREQUENCY is not set, using '1m'")
		pf = "1m"
	}
	pollFreq, err := time.ParseDuration(pf)
	if err != nil {
		log.Printf("failed to parse POLL_FREQUENCY value: %s due to %v, using '1m'", pf, err)
		pollFreq = time.Duration(1 * time.Minute)
	}

	feedType := messages.FeedTypes[os.Getenv("FEED_TYPE")]
	if feedType == "" {
		log.Println("FEED_TYPE is not set, using 'all_hour'")
		feedType = messages.AllHour
	}

	feedsSubject := os.Getenv("FEEDS_SUBJECT") + "." + string(feedType)
	if feedsSubject == "" {
		feedsSubject = "earthquakes.raw." + string(feedType)
		log.Printf("FEEDS_SUBJECT is not set, using '%s'", feedsSubject)
	}

	go fetchFeedsAndPublish(ctx, url, feedsSubject, pollFreq, feedType)

	impactSubject := os.Getenv("IMPACT_SUBJECT")
	if impactSubject == "" {
		impactSubject = "earthquakes.impact"
		log.Printf("IMPACT_SUBJECT is not set, using '%s'", impactSubject)
	}
	// HINT: the poll interval here is fixed because I don't want to change it (unless the Earth becomes an unstable place to live).
	go fetchImpactDataAndPublish(ctx, url, impactSubject, 12*time.Hour, feedType)

	// add goroutine for shutdown via SIGTERM, e.g. so that k8s can cleanly terminate the pod
	// HINT: ideally, the channel creation should be done separately (in main), this is more idiomatic.
	// It removes the minute timing window where SIGTERM arrives but goroutine handling it is not yet registered.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh  // blocks until SIGTERM is received
		cancel() // actual shutdown
	}()

	greetMsg(pollFreq, feedType, feedsSubject)
	<-ctx.Done() // we need to block until all our goroutines finish, main won't wait for them
	log.Println("The Fetcher stopped.")
}
