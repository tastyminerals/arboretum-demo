package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx"
	"github.com/nats-io/nats.go"
	"github.com/tastyminerals/arboretum-demo/services/internal/messages"
	"github.com/tastyminerals/arboretum-demo/services/internal/natsutils"
)

func unmarshalGeoFeature(data []byte) ([]messages.GeoFeature, error) {
	var feats []messages.GeoFeature
	if err := json.Unmarshal(data, &feats); err != nil {
		return feats, fmt.Errorf("unmarshalling received data failed: %w", err)
	}
	return feats, nil
}

func writeToDB(ctx context.Context, pool *sql.DB, feats []messages.GeoFeature) error {

	for _, feat := range feats {
		_, err := pool.ExecContext(ctx, UpsertEventQuery,
			feat.Id,
			feat.Properties.Time,
			feat.Geometry.Coordinates[0], // longitude
			feat.Geometry.Coordinates[1], // latitude
			feat.Properties.Updated,
			feat.Geometry.Coordinates[2], // depth
			feat.Properties.Magnitude,
			feat.Properties.Place,
			feat.Properties.TZ,
			feat.Properties.URL,
			feat.Properties.Detail,
			feat.Properties.Felt,
			feat.Properties.CDI,
			feat.Properties.MMI,
			feat.Properties.Alert,
			feat.Properties.Status,
			feat.Properties.Significance,
			feat.Properties.Network,
			feat.Properties.Code,
			feat.Properties.Ids,
			feat.Properties.Sources,
			feat.Properties.Types,
			feat.Properties.NST,
			feat.Properties.Dmin,
			feat.Properties.RMS,
			feat.Properties.Gap,
			feat.Properties.MagnitudeType,
			feat.Properties.Type,
			feat.Properties.Title,
			feat.Properties.DminDistance,
		)
		if err != nil {
			return fmt.Errorf("failed to upsert event %s: %w", feat.Id, err)
		}
	}
	return nil
}

// Subscribe to NATS, fetch data, quick validate it and write to the database.
func subWithWriteToDB(ctx context.Context, subject string, nc *nats.Conn, pool *sql.DB) error {
	_, err := nc.Subscribe(subject, func(msg *nats.Msg) {
		select {
		case <-ctx.Done():
			return
		default:
			feats, err := unmarshalGeoFeature(msg.Data)
			if err != nil {
				log.Printf("transform failed due to %v", err)
				return
			}
			if err := writeToDB(ctx, pool, feats); err != nil {
				log.Printf("failed to write to db: %+v due to error: %v", feats, err)
			}
		}
	})

	if err != nil {
		return fmt.Errorf("failed to receive a message from %s due to %w", subject, err)
	}

	log.Printf("Subscribed to %s, writing to db\n", subject)
	return nil
}

// check if db is accessible
func pingDB(ctx context.Context, pool *sql.DB, maxTries int) error {
	for try := 0; try < maxTries; try++ {
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		err := pool.PingContext(ctx)
		cancel()

		if err == nil {
			log.Println("connected to db!")
			return nil
		}

		if try < maxTries-1 {
			sleep := time.Duration(1<<try) * time.Second
			log.Printf("db connection attempt %d/%d failed: %v, retrying in %v...", try+1, maxTries, err, sleep)
			time.Sleep(sleep)
		}
	}
	return fmt.Errorf("failed to connect to db after %d attempts", maxTries)
}

func main() {
	// as usual create empty context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// get env vars to use broker messaging
	url := os.Getenv("NATS_URL")
	if url == "" {
		url = "nats://nats:4222"
	}

	eventsSub := os.Getenv("EVENTS_SUBJECT")
	if eventsSub == "" {
		eventsSub = "earthquakes.all_hour"
	}

	nc, err := natsutils.Connect(url, "arboretum-writer")
	if err != nil {
		log.Fatalf("failed to connect to NATS due to %v\n", err)
	}

	// postgresql://user:pass@localhost:5432/mydb
	dsn := os.Getenv("DB_URL")
	if dsn == "" {
		dsn = "postgresql://postgres:letmein@:postgres-service:5432/earthquakes-db"
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("failed to connect to DB due to %v\n", err)
	}
	defer db.Close()

	db.SetConnMaxLifetime(0)
	db.SetMaxIdleConns(2)
	db.SetMaxOpenConns(2)

	if err := pingDB(ctx, db, 3); err != nil {
		log.Fatalf("cannot connect to db unfortunately: %v\n", err)
	}

	if err := subWithWriteToDB(ctx, eventsSub, nc, db); err != nil {
		log.Printf("failed to write to db due to %v", err)
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		if err := nc.Drain(); err != nil {
			log.Printf("failed to drain NATS connections: %v\n", err)
		}
		cancel()
	}()

	log.Println("db writer is ready!")
	<-ctx.Done()
	log.Println("db writer stopped")
}
