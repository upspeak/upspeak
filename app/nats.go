package app

import (
	"fmt"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func startEmbeddedNatsServer(appName string, opts NATSConfig) (*natsserver.Server, error) {
	serverOpts := &natsserver.Options{
		ServerName:      fmt.Sprintf("%s-nats-server", appName),
		DontListen:      opts.Private,
		JetStream:       true,
		JetStreamDomain: appName,
	}

	ns, err := natsserver.NewServer(serverOpts)

	if err != nil {
		return nil, err
	}

	if opts.Logging {
		ns.ConfigureLogger()
	}

	ns.Start()

	if !ns.ReadyForConnections(5 * time.Second) {
		return nil, nats.ErrTimeout
	}

	return ns, nil
}

func connectToEmbeddedNATS(appName string, ns *natsserver.Server, opts NATSConfig) (*nats.Conn, error) {
	clientOpts := []nats.Option{
		nats.Name(fmt.Sprintf("%s-nats-client", appName)),
	}
	if opts.Private {
		clientOpts = append(clientOpts, nats.InProcessServer(ns))
	}
	nc, err := nats.Connect(nats.DefaultURL, clientOpts...)
	if err != nil {
		return nil, err
	}

	return nc, nil
}

func connectToExternalNATS(opts NATSConfig) (*nats.Conn, error) {
	nc, err := nats.Connect(opts.URL)
	if err != nil {
		return nil, err
	}

	return nc, nil
}
