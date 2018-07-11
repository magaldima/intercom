package intercom

import "strconv"

// ServiceAddr implements the net.Addr interface
type ServiceAddr struct {
	Name      string
	Namespace string
	Port      int32
}

// Network for the ServiceAddr - we only operate over tcp
func (addr *ServiceAddr) Network() string {
	return "tcp"
}

// String format of the ServiceAddr
func (addr *ServiceAddr) String() string {
	return addr.Name + ":" + strconv.FormatInt(int64(addr.Port), 10)
}

//rpc error: code = Unavailable desc = all SubConns are in TransientFailure, latest connection error: connection error: desc = "transport: error while dialing: dial tcp: lookup greeter-plugin-svc-8fm77.default: no such host"
