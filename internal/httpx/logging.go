package httpx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

type LoggingTransport struct {
	Base    http.RoundTripper
	Verbose *bool
	Output  io.Writer
}

func (t LoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	started := time.Now()
	res, err := base.RoundTrip(req)
	if t.Verbose == nil || !*t.Verbose {
		return res, err
	}
	output := t.Output
	if output == nil {
		output = io.Discard
	}
	target := req.URL.Host + req.URL.EscapedPath()
	if err != nil {
		fmt.Fprintf(output, "HTTP %s %s failed after %s: %s\n", req.Method, target, time.Since(started).Round(time.Millisecond), Failure(err))
		return nil, err
	}
	fmt.Fprintf(output, "HTTP %s %s -> %d in %s\n", req.Method, target, res.StatusCode, time.Since(started).Round(time.Millisecond))
	return res, nil
}

func Failure(err error) string {
	var networkError net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.As(err, &networkError) && networkError.Timeout():
		return "timeout"
	default:
		return "connection error"
	}
}
