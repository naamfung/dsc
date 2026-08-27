// Copyright IBM Corp. 2016, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/go-plugin/examples/basic/shared"
)

func main() {
	// Create an hclog.Logger
	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "core",
		Output: os.Stdout,
		Level:  hclog.Debug,
	})

	// We're a host! Start by launching the core process.
	client := core.NewClient(&core.ClientConfig{
		HandshakeConfig: handshakeConfig,
		Plugins:         coreMap,
		Cmd:             exec.Command("./core/greeter"),
		Logger:          logger,
	})
	defer client.Kill()

	// Connect via RPC
	rpcClient, err := client.Client()
	if err != nil {
		log.Fatal(err)
	}

	// Request the core
	raw, err := rpcClient.Dispense("greeter")
	if err != nil {
		log.Fatal(err)
	}

	// We should have a Greeter now! This feels like a normal interface
	// implementation but is in fact over an RPC connection.
	greeter := raw.(shared.Greeter)
	fmt.Println(greeter.Greet())
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

// coreMap is the map of plugins we can dispense.
var coreMap = map[string]core.Plugin{
	"greeter": &shared.GreeterPlugin{},
}
