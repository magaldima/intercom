package shared

import (
	"github.com/magaldima/intercom/examples/basic/proto"
	"golang.org/x/net/context"
)

// GRPCClient is an implementation of KV that talks over RPC.
type GRPCClient struct{ client proto.GreeterClient }

// Greet is the implementation of Greeter for a GRPC client
func (m *GRPCClient) Greet() (string, error) {
	resp, err := m.client.Greet(context.Background(), &proto.Empty{})
	if err != nil {
		return "", err
	}
	return resp.Val, nil
}

// Here is the gRPC server that GRPCClient talks to.
type GRPCServer struct {
	// This is the real implementation
	Impl Greeter
}

// Greet is the implementation of Greeter for a GRPC server
func (m *GRPCServer) Greet(context.Context, *proto.Empty) (*proto.Response, error) {
	v, err := m.Impl.Greet()
	return &proto.Response{Val: v}, err
}
