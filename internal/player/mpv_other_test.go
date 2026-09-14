//go:build !windows

package player

import "net"

func listenTestMPV(endpoint string) (net.Listener, error) {
	return net.Listen("unix", endpoint)
}
