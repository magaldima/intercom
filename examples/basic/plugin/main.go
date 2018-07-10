package main

import (
	"fmt"
	"io/ioutil"

	"github.com/magaldima/intercom"
	"github.com/magaldima/intercom/examples/basic/shared"
)

// Here is a real implementation of KV that writes to a local file with
// the key name and the contents are the value of the key.
type KV struct{}

func (KV) Put(key string, value []byte) error {
	value = []byte(fmt.Sprintf("%s\n\nWritten from plugin-go-grpc", string(value)))
	return ioutil.WriteFile("kv_"+key, value, 0644)
}

func (KV) Get(key string) ([]byte, error) {
	return ioutil.ReadFile("kv_" + key)
}

func main() {
	intercom.Serve(&intercom.ServeConfig{
		HandshakeConfig: shared.Handshake,
		Plugins: map[string]intercom.GRPCPlugin{
			"kv": &shared.KVPlugin{Impl: &KV{}},
		},
		GRPCServer: intercom.DefaultGRPCServer,
	})
}
