//go:build !windows

package player

import (
	"context"
	"net"
	"os"
	"path/filepath"
)

func mpvEndpoint() (string, func(), error) {
	dir, err := os.MkdirTemp("", "lemmewatch-mpv-")
	if err != nil {
		return "", nil, err
	}
	return filepath.Join(dir, "ipc"), func() { _ = os.RemoveAll(dir) }, nil
}

func dialMPV(ctx context.Context, endpoint string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", endpoint)
}
