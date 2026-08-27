// Copyright IBM Corp. 2016, 2025
// SPDX-License-Identifier: MPL-2.0

// The core package exposes functions and helpers for communicating to
// plugins which are implemented as standalone binary applications.
//
// core.Client fully manages the lifecycle of executing the application,
// connecting to it, and returning the RPC client for dispensing plugins.
//
// core.Serve fully manages listeners to expose an RPC server from a binary
// that core.Client can connect to.
package core

import (
	"context"
	"errors"
	"net/rpc"

	"google.golang.org/grpc"
)

// Plugin is the interface that is implemented to serve/connect to an
// inteface implementation.
type Plugin interface {
	// Server should return the RPC server compatible struct to serve
	// the methods that the Client calls over net/rpc.
	Server(*MuxBroker) (interface{}, error)

	// Client returns an interface implementation for the core you're
	// serving that communicates to the server end of the core.
	Client(*MuxBroker, *rpc.Client) (interface{}, error)
}

// GRPCPlugin is the interface that is implemented to serve/connect to
// a core over gRPC.
type GRPCPlugin interface {
	// GRPCServer should register this core for serving with the
	// given GRPCServer. Unlike Plugin.Server, this is only called once
	// since gRPC plugins serve singletons.
	GRPCServer(*GRPCBroker, *grpc.Server) error

	// GRPCClient should return the interface implementation for the core
	// you're serving via gRPC. The provided context will be canceled by
	// go-core in the event of the core process exiting.
	GRPCClient(context.Context, *GRPCBroker, *grpc.ClientConn) (interface{}, error)
}

// NetRPCUnsupportedPlugin implements Plugin but returns errors for the
// Server and Client functions. This will effectively disable support for
// net/rpc based plugins.
//
// This struct can be embedded in your struct.
type NetRPCUnsupportedPlugin struct{}

func (p NetRPCUnsupportedPlugin) Server(*MuxBroker) (interface{}, error) {
	return nil, errors.New("net/rpc core protocol not supported")
}

func (p NetRPCUnsupportedPlugin) Client(*MuxBroker, *rpc.Client) (interface{}, error) {
	return nil, errors.New("net/rpc core protocol not supported")
}
