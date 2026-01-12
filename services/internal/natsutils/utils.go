package natsutils

import (
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

// Connect to NATS broker using some preconfigured defaults without any authentication.
func Connect(url string, clientName string) (*nats.Conn, error) {
	nc, err := nats.Connect(url,
		nats.Name(clientName),
		nats.MaxReconnects(-1), // infinite
		nats.ReconnectWait(2*time.Second),
		nats.ReconnectJitter(500*time.Millisecond, 2*time.Second),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			log.Printf("NATS disconnect due to %v\n", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("NATS reconnected to %s\n", nc.ConnectedUrl())
		}),
		nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
			log.Printf("NATS async error due to %v\n", err)
		}),
		nats.Timeout(10*time.Second),
		nats.ReconnectBufSize(1*1024*1024), // 1MB instead of default 8MB, we have low publish frequency
	)

	return nc, err
}
