package nats

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/upspeak/upspeak/app"
)

// Config defines the configuration for the NATS infrastructure.
type Config struct {
	URL      string `mapstructure:"url"`
	Embedded bool   `mapstructure:"embedded"`
	Private  bool   `mapstructure:"private"`
	Logging  bool   `mapstructure:"logging"`
}

// Bus manages the NATS connection and optional embedded server.
// It provides Publisher and Subscriber implementations for the app framework.
type Bus struct {
	nc     *nats.Conn
	ns     *server.Server
	logger *slog.Logger
}

// Start creates and starts a new NATS Bus. If config.Embedded is true, an
// in-process NATS server with JetStream is started. Otherwise, connects to
// the external NATS server at config.URL.
func Start(appName string, config Config) (*Bus, error) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	bus := &Bus{logger: logger}

	if config.Embedded {
		ns, err := startEmbeddedServer(appName, config)
		if err != nil {
			return nil, fmt.Errorf("failed to start embedded NATS server: %w", err)
		}
		bus.ns = ns
		logger.Info("Started embedded NATS server", "name", appName)

		nc, err := connectToEmbedded(appName, ns, config)
		if err != nil {
			ns.Shutdown()
			return nil, fmt.Errorf("failed to connect to embedded NATS server: %w", err)
		}
		bus.nc = nc
		logger.Info("Connected to embedded NATS server", "private", config.Private)
	} else {
		nc, err := nats.Connect(config.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to NATS server at %s: %w", config.URL, err)
		}
		bus.nc = nc
		logger.Info("Connected to external NATS server", "url", config.URL)
	}

	return bus, nil
}

// Stop gracefully shuts down the NATS connection and embedded server.
func (b *Bus) Stop() {
	b.logger.Info("Stopping NATS...")
	if b.nc != nil {
		b.nc.Close()
		b.logger.Info("NATS connection closed")
	}
	if b.ns != nil {
		b.ns.Shutdown()
		b.ns.WaitForShutdown()
		b.logger.Info("Embedded NATS server stopped")
	}
}

// Conn returns the underlying NATS connection.
func (b *Bus) Conn() *nats.Conn {
	return b.nc
}

// Publisher returns a Publisher that implements app.Publisher.
func (b *Bus) Publisher() app.Publisher {
	return &publisher{nc: b.nc}
}

// Subscriber returns a Subscriber that implements app.Subscriber.
func (b *Bus) Subscriber() app.Subscriber {
	return &subscriber{nc: b.nc}
}

func startEmbeddedServer(appName string, config Config) (*server.Server, error) {
	opts := &server.Options{
		ServerName:      fmt.Sprintf("%s-nats-server", appName),
		DontListen:      config.Private,
		JetStream:       true,
		JetStreamDomain: appName,
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		return nil, err
	}

	if config.Logging {
		ns.ConfigureLogger()
	}

	ns.Start()

	if !ns.ReadyForConnections(5 * time.Second) {
		return nil, nats.ErrTimeout
	}

	return ns, nil
}

func connectToEmbedded(appName string, ns *server.Server, config Config) (*nats.Conn, error) {
	clientOpts := []nats.Option{
		nats.Name(fmt.Sprintf("%s-nats-client", appName)),
	}
	if config.Private {
		clientOpts = append(clientOpts, nats.InProcessServer(ns))
	}
	return nats.Connect(nats.DefaultURL, clientOpts...)
}
