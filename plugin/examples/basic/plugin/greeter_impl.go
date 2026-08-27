// Copyright IBM Corp. 2016, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"os"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/go-plugin/examples/basic/shared"
)

// Here is a real implementation of Greeter
type GreeterHello struct {
	logger hclog.Logger
}

func (g *GreeterHello) Greet() string {
	g.logger.Debug("message from GreeterHello.Greet")
	return "Hello!"
}

// handshakeConfigs are used to just do a basic handshake between
// a core and host. If the handshake fails, a user friendly error is shown.
// This prevents users from executing bad plugins or executing a core
// directory. It is a UX feature, not a security feature.
var handshakeConfig = core.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "BASIC_PLUGIN",
	MagicCookieValue: "hello",
}

func main() {
	logger := hclog.New(&hclog.LoggerOptions{
		Level:      hclog.Trace,
		Output:     os.Stderr,
		JSONFormat: true,
	})

	greeter := &GreeterHello{
		logger: logger,
	}
	// coreMap is the map of plugins we can dispense.
	var coreMap = map[string]core.Plugin{
		"greeter": &shared.GreeterPlugin{Impl: greeter},
	}

	logger.Debug("message from core", "foo", "bar")

	core.Serve(&core.ServeConfig{
		HandshakeConfig: handshakeConfig,
		Plugins:         coreMap,
	})
}
