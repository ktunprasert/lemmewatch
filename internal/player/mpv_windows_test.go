//go:build windows

package player

import (
	"net"

	"github.com/Microsoft/go-winio"
)

func listenTestMPV(endpoint string) (net.Listener, error) {
	return winio.ListenPipe(endpoint, nil)
}
