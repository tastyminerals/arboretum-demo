// This package implements a service that queries USGS API for up-to-date earthquake data.
// It is has a boring name: "fetcher".
// The fetcher spawns a goroute to fetch + push JSON data to our message broker.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tastyminerals/arboretum-demo/services/internal/natsutils"
)

var feedsUrl = "https://earthquake.usgs.gov/earthquakes/feed/v1.0/summary/"

// Pring a fetcher greeting message with some useful info
func greetMsg(pollFreq time.Duration, feedType string, subject string) {
	log.Printf("---> Hello there, the Fetcher is online! <---")
	log.Printf("We shall be polling at the speed of %s from %s and publishing to %s", pollFreq.String(), feedType, subject)
}

// Create http client and perform a GET request to fetch USGS feed data.
func fetchData(ctx context.Context, feedType string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedsUrl+feedType, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	// response.Body.Close() should be err checked, wrap it into deferable function
	defer func() {
		if err := response.Body.Close(); err != nil {
			log.Printf("failed to close response body because of %v", err)
		}
	}()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected API response status: %d", response.StatusCode)
	}
	// in Go, the response Body is streamed on demand, we need a explicitly read from it
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// This function fetches the USGS feed as JSON once per poll frequency set in the manifest and pushes it to the message broker.
// The function performs up to 5 retries with simple backoff.
// HINT: The update frequency is set to once per minute because this is the frequency of USGS feeds update.
func fetchAndPublish(ctx context.Context, url string, subject string, pollFreq time.Duration, feedType string) {

	// create unauthenticated NATS connection
	nc, err := natsutils.Connect(url, "arboretum-fetcher")
	if err != nil {
		log.Fatalf("failed to connect to NATS due to %v\n", err)
	}

	defer func() {
		if err := nc.Drain(); err != nil {
			log.Printf("failed to drain NATS connection: %v", err)
		}
	}()

	ticker := time.NewTicker(pollFreq)
	defer ticker.Stop() // Go ticker never stops, we need to defer stop it

	// start the main fetcher loop that queries USGS and publishes []bytes to broker
	for {
		select {
		case <-ticker.C:
			var data []byte
			var err error
			maxRetries := 5

			requestCtx, requestCancel := context.WithTimeout(ctx, 10*time.Second)

			for attempt := 0; attempt < maxRetries; attempt++ {
				data, err = fetchData(requestCtx, feedType+".geojson")
				if err == nil {
					break
				}

				fmt.Println("Next attempt")
				if attempt < maxRetries-1 {
					log.Printf("attempt %d failed to fetch data because of %v", attempt+1, err)
					// double the sleep time with every attempt: 1s, 2s, 4s, 8s, 16s
					sleep := int64(1000 * (1 << attempt))
					// add random jitter seeded upon package import
					jitter := rand.Int63n(sleep)
					time.Sleep(time.Duration(sleep+jitter) * time.Millisecond)
				}
			}

			requestCancel() // precaution explicit canceling even if we have timeout set

			if err != nil {
				log.Printf("fetching data failed after %d attempts because of %v\n", maxRetries, err)
				continue
			}

			// if nothing subscribed to this channel, the message won't be delivered
			if err := nc.Publish(subject, data); err != nil {
				log.Printf("failed to publish to %s because of %v", subject, err)
			} else {
				log.Printf("published %.2fKB message to %s", float64(len(data))/1024, subject)
			}

		case <-ctx.Done(): // triggered when SIGTERM is called: the parent context will propagate here
			return
		}
	}
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

	feedType := os.Getenv("FEED_FREQUENCY_TYPE")
	if feedType == "" {
		log.Println("FEED_FREQUENCY_TYPE is not set, using 'all_hour'")
		feedType = "all_hour"
	}

	pubSubject := os.Getenv("PUB_SUBJECT") + "." + feedType
	if pubSubject == "" {
		pubSubject = "earthquakes.raw." + feedType
		log.Printf("PUB_SUBJECT is not set, using '%s'", pubSubject)
	}

	go fetchAndPublish(ctx, url, pubSubject, pollFreq, feedType)

	// add goroutine for shutdown via SIGTERM, e.g. so that k8s can cleanly terminate the pod
	// HINT: ideally, the channel creation should be done separately (in main), this is more idiomatic.
	// It removes the minute timing window where SIGTERM arrives but goroutine handling it is not yet registered.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh  // blocks until SIGTERM is received
		cancel() // actual shutdown
	}()

	greetMsg(pollFreq, feedType, pubSubject)
	<-ctx.Done() // we need to block until all our goroutines finish, main won't wait for them
	log.Println("The Fetcher stopped.")
}
