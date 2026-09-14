package player

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"runtime"
	"strconv"
)

func newVLCTracker() ([]string, *progressTracker, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return nil, nil, err
	}
	password := rand.Text()
	transport := &http.Transport{}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	endpoint := "http://127.0.0.1:" + strconv.Itoa(port) + "/requests/status.json"
	tracker := &progressTracker{
		poll: func(ctx context.Context) (Progress, error) {
			return pollVLC(ctx, client, endpoint, password)
		},
		close: transport.CloseIdleConnections,
	}
	flags := []string{
		"--no-one-instance", "--no-one-instance-when-started-from-file",
		"--extraintf=http", "--http-host=127.0.0.1", "--http-port=" + strconv.Itoa(port),
		"--http-password=" + password,
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
		flags = append(flags, "--qt-continue=0")
	}
	return flags, tracker, nil
}

func pollVLC(ctx context.Context, client *http.Client, endpoint, password string) (Progress, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Progress{}, err
	}
	req.SetBasicAuth("", password)
	response, err := client.Do(req)
	if err != nil {
		return Progress{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Progress{}, errors.New("VLC status unavailable")
	}
	var status struct {
		Time   *float64 `json:"time"`
		Length float64  `json:"length"`
		State  string   `json:"state"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&status); err != nil {
		return Progress{}, err
	}
	if status.Time == nil || (status.State != "playing" && status.State != "paused") {
		return Progress{}, errors.New("VLC has no active playback")
	}
	return Progress{Position: *status.Time, Duration: status.Length}, nil
}
