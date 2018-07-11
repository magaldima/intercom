package intercom

import (
	"context"
	"crypto/tls"
	fmt "fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// GRPCClient connects to a GRPCServer over gRPC to dispense plugin types.
type GRPCClient struct {
	Conn    *grpc.ClientConn
	Plugins map[string]GRPCPlugin

	doneCtx context.Context
	broker  *GRPCBroker
}

// newGRPCClient creates a new GRPCClient. The Client argument is expected
// to be successfully started already with a lock held.
func newGRPCClient(doneCtx context.Context, c *Client) (*GRPCClient, error) {
	conn, err := dialGRPCConn(c.config.TLSConfig, c.dialer)
	if err != nil {
		return nil, err
	}

	// Start the broker.
	brokerGRPCClient := newGRPCBrokerClient(conn)
	broker := newGRPCBroker(brokerGRPCClient, c.config.TLSConfig)
	go broker.Run()
	go brokerGRPCClient.StartStream()

	return &GRPCClient{
		Conn:    conn,
		Plugins: c.config.Plugins,
		doneCtx: doneCtx,
		broker:  broker,
	}, nil
}

// Close the ClientProtocol impl.
func (c *GRPCClient) Close() error {
	c.broker.Close()
	return c.Conn.Close()
}

// Dispense from ClientProtocol impl.
func (c *GRPCClient) Dispense(name string) (interface{}, error) {
	raw, ok := c.Plugins[name]
	if !ok {
		return nil, fmt.Errorf("unknown plugin type: %s", name)
	}

	return raw.GRPCClient(c.doneCtx, c.broker, c.Conn)
}

// Ping the ClientProtocol impl.
func (c *GRPCClient) Ping() error {
	client := grpc_health_v1.NewHealthClient(c.Conn)
	_, err := client.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{
		Service: GRPCServiceName,
	})

	return err
}

func dialGRPCConn(tls *tls.Config, dialer func(string, time.Duration) (net.Conn, error)) (*grpc.ClientConn, error) {
	// Build dialing options.
	opts := make([]grpc.DialOption, 0, 5)

	// We use a custom dialer so that we can connect over unix domain sockets
	opts = append(opts, grpc.WithDialer(dialer))

	// we add a timeout
	opts = append(opts, grpc.WithTimeout(5*time.Second))

	// go-plugin expects to block the connection
	opts = append(opts, grpc.WithBlock())

	// Fail right away
	opts = append(opts, grpc.FailOnNonTempDialError(true))

	// If we have no TLS configuration set, we need to explicitly tell grpc
	// that we're connecting with an insecure connection.
	if tls == nil {
		opts = append(opts, grpc.WithInsecure())
	} else {
		opts = append(opts, grpc.WithTransportCredentials(
			credentials.NewTLS(tls)))
	}

	// Connect. Note the first parameter is unused because we use a custom
	// dialer that has the state to see the address.
	conn, err := grpc.Dial("unused", opts...)
	if err != nil {
		return nil, err
	}

	return conn, nil
}
