//go:build windows

package player

import (
	"context"
	"crypto/rand"
	"net"

	"github.com/Microsoft/go-winio"
)

func mpvEndpoint() (string, func(), error) {
	return `\\.\pipe\lemmewatch-mpv-` + rand.Text(), func() {}, nil
}

func dialMPV(ctx context.Context, endpoint string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, endpoint)
}
