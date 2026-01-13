// This package implements a service that unmarshals, transforms and converts the fetcher results.
// The converted data is published to NATS for downstream subscribers.
// The transformer should handle missing or corrupted data.
// The transformer must not publish incomplete or corrupted data.
package main

import (
	"context"
	_ "embed"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

//go:embed assets/contributors.tsv
var contributorsTSV string

// Initialize contributors map for the "net" field expansion from embedded tsv file.
func initContributors() (map[string]string, error) {
	contributors := make(map[string]string)
	reader := csv.NewReader(strings.NewReader(contributorsTSV))
	reader.Comma = '\t'

	rows, err := reader.ReadAll()
	if err != nil {
		return contributors, fmt.Errorf("failed to read contributors.tsv because of %w", err)
	}

	for _, row := range rows {
		if len(row) >= 2 {
			contributors[row[0]] = row[1]
		}
	}
	return contributors, nil
}

func main() {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	contributors, err := initContributors()
	if err != nil {
		log.Fatalf("cannot initialize contributors map due to %v", err)
	}

	url := os.Getenv("NATS_URL")
	if url == "" {
		url = "nats://nats:4222"
	}

	// we subscribe to the fetcher PUB_SUBJECT
	subSubject := os.Getenv("SUB_SUBJECT")
	if subSubject == "" {
		log.Println("SUB_SUBJECT is not set! Using 'earthquakes.raw.all_hour'")
		subSubject = "earthquakes.raw.all_hour"
	}

	// we publish data after transform to a different subject
	pubSubject := os.Getenv("PUB_SUBJECT")
	if pubSubject == "" {
		log.Println("PUB_SUBJECT is not set! Using 'earthquakes.all_hour'")
		pubSubject = "earthquakes.all_hour"
	}

	transformer := NewTransformer(contributors, subSubject, pubSubject, url)
	defer func() {
		if err := transformer.nc.Drain(); err != nil {
			log.Printf("failed to drain NATS connections: %v\n", err)
		}
	}()

	// here we register a func callback that will be triggered whenver a message arrives
	err = transformer.subWithTransformAndPublishCallback(ctx)

	if err != nil {
		log.Fatalf("subscription failed due to %v", err)
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		cancel()
	}()

	log.Println("For better or worse, transform you...")
	<-ctx.Done()
	log.Println("Transformer stopped")
}
