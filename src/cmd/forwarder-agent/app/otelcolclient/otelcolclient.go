// Package otelcolclient contains client code for communicating with an OTel
// Collector.
package otelcolclient

import (
	"context"
	"log"

	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	// The client API for the OTel Collector metrics service.
	msc colmetricspb.MetricsServiceClient
	// The logger to use for errors
	l *log.Logger
}

// New dials the provided gRPC address and returns a *Client or error based off
// that client connection.
func New(addr string, l *log.Logger) (*Client, error) {
	// TODO: setup real credentials
	cc, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return &Client{msc: colmetricspb.NewMetricsServiceClient(cc), l: l}, nil
}

// writeCounter translates a loggregator v2 Counter to OTLP and forwards it.
func (c *Client) writeCounter(e *loggregator_v2.Envelope) error {
	_, err := c.msc.Export(context.TODO(), &colmetricspb.ExportMetricsServiceRequest{})
	if err != nil {
		return err
	}
	return nil
}

// writeGauge translates a loggregator v2 Gauge to OTLP and forwards it.
func (c *Client) writeGauge(e *loggregator_v2.Envelope) error {
	_, err := c.msc.Export(context.TODO(), &colmetricspb.ExportMetricsServiceRequest{})
	if err != nil {
		return err
	}
	return nil
}

// Write translates an envelope to OTLP and forwards it to the connected OTel
// Collector.
func (c *Client) Write(e *loggregator_v2.Envelope) error {
	var err error
	switch e.Message.(type) {
	case *loggregator_v2.Envelope_Counter:
		err = c.writeCounter(e)
	case *loggregator_v2.Envelope_Gauge:
		err = c.writeGauge(e)
	}
	if err != nil {
		c.l.Println("Write error:", err)
	}
	return err
}

// Close flushes any buffers and closes any connections.
func (c *Client) Close() error {
	return nil
}
