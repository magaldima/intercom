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
	if addr.Namespace != "" {
		return addr.Name + "." + addr.Namespace + ":" + strconv.FormatInt(int64(addr.Port), 10)
	}
	return addr.Name + ":" + strconv.FormatInt(int64(addr.Port), 10)
}
