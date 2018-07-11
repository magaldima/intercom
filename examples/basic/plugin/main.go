package main

import (
	"github.com/magaldima/intercom"
	"github.com/magaldima/intercom/examples/basic/shared"
)

// Greeter is a real implementation of Greeter that simply prints hello, world
type Greeter struct{}

// Greet with hello, world message
func (Greeter) Greet() (string, error) {
	return "hello, world", nil
}

func main() {
	intercom.Serve(&intercom.ServeConfig{
		HandshakeConfig: shared.Handshake,
		Plugins: map[string]intercom.GRPCPlugin{
			"greeter": &shared.GreeterPlugin{Impl: &Greeter{}},
		},
		GRPCServer: intercom.DefaultGRPCServer,
	})
}
