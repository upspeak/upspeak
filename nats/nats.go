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

// Bus manages the NATS connection, optional embedded server, and JetStream context.
// It provides Publisher, Subscriber, and Consumer implementations for the app framework.
type Bus struct {
	nc     *nats.Conn
	js     nats.JetStreamContext
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

		nc, err := connectToEmbedded(appName, ns, config, logger)
		if err != nil {
			ns.Shutdown()
			return nil, fmt.Errorf("failed to connect to embedded NATS server: %w", err)
		}
		bus.nc = nc
		logger.Info("Connected to embedded NATS server", "private", config.Private)
	} else {
		nc, err := connectToExternal(appName, config, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to NATS server at %s: %w", config.URL, err)
		}
		bus.nc = nc
		logger.Info("Connected to external NATS server", "url", config.URL)
	}

	// Obtain JetStream context for the connection.
	js, err := bus.nc.JetStream()
	if err != nil {
		bus.nc.Close()
		if bus.ns != nil {
			bus.ns.Shutdown()
		}
		return nil, fmt.Errorf("failed to get JetStream context: %w", err)
	}
	bus.js = js

	return bus, nil
}

// Stop gracefully drains the NATS connection and shuts down the embedded server.
// Drain unsubscribes all subscriptions, waits for in-flight message handlers to
// complete, flushes the publish buffer, then closes the connection.
func (b *Bus) Stop() {
	b.logger.Info("Stopping NATS...")
	if b.nc != nil {
		if err := b.nc.Drain(); err != nil {
			b.logger.Error("Failed to drain NATS connection, forcing close", "error", err)
			b.nc.Close()
		} else {
			b.logger.Info("NATS connection drained")
		}
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

// JetStream returns the JetStream context for the connection.
func (b *Bus) JetStream() nats.JetStreamContext {
	return b.js
}

// Publisher returns a Publisher that implements app.Publisher using JetStream.
func (b *Bus) Publisher() app.Publisher {
	return &publisher{js: b.js}
}

// Subscriber returns a Subscriber that implements app.Subscriber.
func (b *Bus) Subscriber() app.Subscriber {
	return &subscriber{nc: b.nc}
}

// connectionHandlers returns common NATS connection option handlers for logging.
func connectionHandlers(logger *slog.Logger) []nats.Option {
	return []nats.Option{
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				logger.Warn("NATS disconnected", "error", err)
			} else {
				logger.Info("NATS disconnected")
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Info("NATS reconnected", "url", nc.ConnectedUrl())
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			if err := nc.LastError(); err != nil {
				logger.Error("NATS connection closed", "error", err)
			} else {
				logger.Info("NATS connection closed")
			}
		}),
		nats.ErrorHandler(func(_ *nats.Conn, sub *nats.Subscription, err error) {
			if sub != nil {
				logger.Error("NATS async error", "subject", sub.Subject, "error", err)
			} else {
				logger.Error("NATS async error", "error", err)
			}
		}),
	}
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

func connectToEmbedded(appName string, ns *server.Server, config Config, logger *slog.Logger) (*nats.Conn, error) {
	opts := []nats.Option{
		nats.Name(fmt.Sprintf("%s-nats-client", appName)),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(500 * time.Millisecond),
		nats.ReconnectBufSize(8 << 20), // 8 MB
	}
	opts = append(opts, connectionHandlers(logger)...)

	if config.Private {
		opts = append(opts, nats.InProcessServer(ns))
	}
	return nats.Connect(nats.DefaultURL, opts...)
}

func connectToExternal(appName string, config Config, logger *slog.Logger) (*nats.Conn, error) {
	opts := []nats.Option{
		nats.Name(fmt.Sprintf("%s-nats-client", appName)),
		nats.Timeout(5 * time.Second),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
		nats.ReconnectJitter(100*time.Millisecond, time.Second),
		nats.ReconnectBufSize(8 << 20), // 8 MB
	}
	opts = append(opts, connectionHandlers(logger)...)

	return nats.Connect(config.URL, opts...)
}
