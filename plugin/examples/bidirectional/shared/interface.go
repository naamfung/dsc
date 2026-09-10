// Copyright IBM Corp. 2016, 2025
// SPDX-License-Identifier: MPL-2.0

// Package shared contains shared data between the host and plugins.
package shared

import (
	"context"

	"google.golang.org/grpc"

	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/go-plugin/examples/bidirectional/proto"
)

// Handshake is a common handshake that is shared by core and host.
var Handshake = core.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "BASIC_PLUGIN",
	MagicCookieValue: "hello",
}

// PluginMap is the map of plugins we can dispense.
var PluginMap = map[string]core.Plugin{
	"counter": &CounterPlugin{},
}

type AddHelper interface {
	Sum(int64, int64) (int64, error)
}

// Counter is the interface that we're exposing as a core.
type Counter interface {
	Put(key string, value int64, a AddHelper) error
	Get(key string) (int64, error)
}

// This is the implementation of core.Plugin so we can serve/consume this.
// We also implement GRPCPlugin so that this core can be served over
// gRPC.
type CounterPlugin struct {
	core.NetRPCUnsupportedPlugin
	// Concrete implementation, written in Go. This is only used for plugins
	// that are written in Go.
	Impl Counter
}

func (p *CounterPlugin) GRPCServer(broker *core.GRPCBroker, s *grpc.Server) error {
	proto.RegisterCounterServer(s, &GRPCServer{
		Impl:   p.Impl,
		broker: broker,
	})
	return nil
}

func (p *CounterPlugin) GRPCClient(ctx context.Context, broker *core.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &GRPCClient{
		client: proto.NewCounterClient(c),
		broker: broker,
	}, nil
}

var _ core.GRPCPlugin = &CounterPlugin{}
