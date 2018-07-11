// Package shared contains shared data between the host and plugins.
package shared

import (
	"context"

	"google.golang.org/grpc"

	"github.com/magaldima/intercom"
	"github.com/magaldima/intercom/examples/basic/proto"
)

// Handshake is a common handshake that is shared by plugin and host.
var Handshake = intercom.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "BASIC_PLUGIN",
	MagicCookieValue: "hello",
}

// PluginMap is the map of plugins we can dispense.
var PluginMap = map[string]intercom.GRPCPlugin{
	"greeter": &GreeterPlugin{},
}

// Greeter is the interface that we're exposing as a plugin.
type Greeter interface {
	Greet() (string, error)
}

// This is the implementation of plugin.Plugin so we can serve/consume this.
// We also implement GRPCPlugin so that this plugin can be served over
// gRPC.
type GreeterPlugin struct {
	// Concrete implementation, written in Go. This is only used for plugins
	// that are written in Go.
	Impl Greeter
}

func (p *GreeterPlugin) GRPCServer(broker *intercom.GRPCBroker, s *grpc.Server) error {
	proto.RegisterGreeterServer(s, &GRPCServer{Impl: p.Impl})
	return nil
}

func (p *GreeterPlugin) GRPCClient(ctx context.Context, broker *intercom.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &GRPCClient{client: proto.NewGreeterClient(c)}, nil
}
