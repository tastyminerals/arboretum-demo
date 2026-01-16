package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/tastyminerals/arboretum-demo/services/internal/messages"
)

// Pring a fetcher greeting message with some useful info
func greetMsg(pollFreq time.Duration, feedType messages.Feed, subject string) {
	log.Printf("---> Hello there, the Fetcher is online! <---")
	log.Printf("We shall be polling at the speed of %s from %s and publishing to %s", pollFreq.String(), string(feedType), subject)
}

// Perform http GET request with error handling and reading the response body.
func httpGet(ctx context.Context, url string, timeout int) ([]byte, error) {
	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close response body due to %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected API response status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Wrapper function that calls provided data fetching func and nats publisher indefinitely using specified time interval with retries.
func withPolling(ctx context.Context, fun func(context.Context) ([][]byte, error), nc *nats.Conn, subject string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop() // Go ticker never stops, we need to defer stop it

	// start the main fetcher loop that queries USGS and publishes []bytes to broker
	for {
		select {
		case <-ticker.C:
			var bbytes [][]byte
			var err error
			maxRetries := 5

			requestCtx, requestCancel := context.WithTimeout(ctx, 10*time.Second)

			for attempt := 0; attempt < maxRetries; attempt++ {

				bbytes, err = fun(requestCtx) // data fetching function

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

			// Now, iterate over [][]bytes array and publish each bytes to NATS.
			// HINT: Why do we stick to [][]bytes here? This is because we want to use both feeds AND detail fetching data functions with withPolling function.
			for _, bytes := range bbytes {
				// if nothing subscribed to this channel, the message won't be delivered
				if err := nc.Publish(subject, bytes); err != nil {
					log.Printf("failed to publish to %s because of %v", subject, err)
				} else {
					log.Printf("published %.2fKB message to %s", float64(len(bytes))/1024, subject)
				}
			}

		case <-ctx.Done(): // triggered when SIGTERM is called: the parent context will propagate here
			return
		}
	}
}
