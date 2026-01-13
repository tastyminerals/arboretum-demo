package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/tastyminerals/arboretum-demo/services/internal/natsutils"
)

// Earth circumference 40075 km / 360 degrees; 1 degree ~ 111.32 km
var degInKM = 111.32

func isValidFloat(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

// Not your attention transformer or Takara transformer...
// This is just a data struct so you don't pollute the function signatures too much.
type Transformer struct {
	contributors map[string]string
	subSubject   string
	pubSubject   string
	nc           *nats.Conn
}

// a function to create a Transformer cla#*, oops, go struct instance
func NewTransformer(contributors map[string]string, subSubject, pubSubject string, url string) *Transformer {
	nc, err := natsutils.Connect(url, "arboretum-transformer")
	if err != nil {
		log.Fatalf("failed to connect to NATS due to %v\n", err)
	}

	return &Transformer{
		contributors: contributors,
		subSubject:   subSubject,
		pubSubject:   pubSubject,
		nc:           nc,
	}
}

// Subscribe, transform and publish the transfomed message to NATS.
func (t *Transformer) transformAndPublish(ctx context.Context) error {
	// Register message listening callback and return, no need to block with <-ctx.Done here, we have it in main
	// we use async subscribe and a message handler callback that is executed upon message arrival
	_, err := t.nc.Subscribe(t.subSubject, func(msg *nats.Msg) {
		select {
		case <-ctx.Done(): // if we received SIGTERM don't run transform, just exit
			return
		default:
		}

		transformed, err := t.transform(msg.Data)
		fmt.Printf("transformed data --> %s\n", string(transformed))
		if err != nil {
			log.Printf("transform failed due to %v", err)
			return
		}

		if err := t.nc.Publish(t.pubSubject, transformed); err != nil {
			log.Printf("failed to publish transformed feeds due to %v", err)
		}
	})

	if err != nil {
		return fmt.Errorf("failed to receive a message from %s because of %w", t.subSubject, err)
	}

	log.Printf("Subscribed to %s, publishing to %s\n", t.subSubject, t.pubSubject)
	return nil
}

func (t *Transformer) transform(data []byte) ([]byte, error) {
	var feeds USGSFeeds

	if err := json.Unmarshal(data, &feeds); err != nil {
		return nil, fmt.Errorf("unmarshalling received data failed: %w", err)
	}

	features, err := t.convert(feeds.Features)
	if err != nil {
		return nil, fmt.Errorf("converting geo features failed: %w", err)
	}

	featuresAsData, err := json.Marshal(features)
	if err != nil {
		return nil, fmt.Errorf("marshaling geo features failed: %w", err)
	}

	return featuresAsData, nil
}

// Perform some custom field convertions:
//
//   - "net": expand "net" abbreviation using contributors.tsv asset
//   - "dmin_distance": use "dmin" value to calculate the distance to the closes station in km
//   - "ids", "sources", "types": clean-up leading and trailing commas
//
// TIP: keep error for future more sensitive data convertions
func (t *Transformer) convert(features []GeoFeature) ([]GeoFeature, error) {
	// TIP: index only based iteration is used here because Go range creates value copies if they are also requested
	for i := range features {
		// expand abbreviations if key exists, otherwise, we keep the original "net" value
		if expanded, ok := t.contributors[features[i].Properties.Network]; ok {
			features[i].Properties.Network = expanded
		}
		// add the distance to nearest station in km
		if isValidFloat(features[i].Properties.Dmin) {
			features[i].Properties.DminDistance = features[i].Properties.Dmin * degInKM
		}

		// clean-up leading and trailing commas in certain fields
		features[i].Properties.Ids = strings.Trim(features[i].Properties.Ids, ",")
		features[i].Properties.Sources = strings.Trim(features[i].Properties.Sources, ",")
		features[i].Properties.Types = strings.Trim(features[i].Properties.Types, ",")
	}
	return features, nil
}
