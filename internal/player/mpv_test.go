package player

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func readMPVSubscriptions(conn net.Conn) error {
	decoder := json.NewDecoder(conn)
	for index, name := range []string{"duration", "time-pos"} {
		var command struct {
			Command   []any `json:"command"`
			RequestID int   `json:"request_id"`
		}
		if err := decoder.Decode(&command); err != nil {
			return err
		}
		if len(command.Command) != 3 || command.Command[0] != "observe_property" || command.Command[1] != float64(index+1) || command.Command[2] != name || command.RequestID != index+1 {
			return fmt.Errorf("unexpected subscription: %#v", command)
		}
	}
	return nil
}

func TestMPVObservationsFlushLatestPositionOnDisconnect(t *testing.T) {
	for _, test := range []struct {
		name   string
		events []string
		want   Progress
	}{
		{"forward seek", []string{
			`{"event":"property-change","name":"duration","data":1800}`,
			`{"event":"property-change","name":"time-pos","data":100}`,
			`{"event":"property-change","name":"time-pos","data":723.75}`,
			`{"event":"property-change","name":"time-pos","data":null}`,
			`{"event":"property-change","name":"duration"}`,
		}, Progress{723.75, 1800}},
		{"backward seek and unknown duration", []string{
			`{"error":"success","request_id":1}`,
			`{"event":"property-change","name":"duration","data":null}`,
			`{"event":"property-change","name":"time-pos","data":700}`,
			`{"event":"property-change","name":"time-pos","data":20.25}`,
			`{"event":"property-change","name":"media-title","data":"unrelated"}`,
			`{"event":"end-file","reason":"stop"}`,
		}, Progress{20.25, 0}},
		{"completed", []string{
			`{"event":"property-change","name":"duration","data":1800}`,
			`{"event":"property-change","name":"time-pos","data":1790}`,
			`{"event":"end-file","reason":"eof"}`,
			`{"event":"shutdown"}`,
		}, Progress{1800, 1800}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer server.Close()
			_ = server.SetDeadline(time.Now().Add(3 * time.Second))
			done := make(chan error, 1)
			go func() {
				defer server.Close()
				if err := readMPVSubscriptions(server); err != nil {
					done <- err
					return
				}
				_, err := io.WriteString(server, strings.Join(test.events, "\n")+"\n")
				done <- err
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var got Progress
			err := observeMPV(ctx, client, func(p Progress) { got = p }, time.Hour)
			if !errors.Is(err, io.EOF) || got != test.want {
				t.Fatalf("final checkpoint = %#v, %v; want %#v", got, err, test.want)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMPVObservationsFlushOnCancellation(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	_ = server.SetDeadline(time.Now().Add(3 * time.Second))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := progressTracker{track: func(ctx context.Context, save func(Progress)) {
		_ = observeMPV(ctx, client, save, time.Hour)
	}}
	var got Progress
	stop := tracker.start(ctx, func(p Progress) { got = p })
	defer stop()
	if err := readMPVSubscriptions(server); err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{
		`{"event":"property-change","name":"time-pos","data":10}`,
		`{"event":"property-change","name":"time-pos","data":22.75}`,
		`{"event":"idle"}`,
	} {
		if _, err := fmt.Fprintln(server, event); err != nil {
			t.Fatal(err)
		}
	}
	// Reading the idle event means the preceding position was delivered to the tracker.
	stop()
	if got.Position != 22.75 {
		t.Fatalf("cancellation lost latest checkpoint: %#v", got)
	}
}

func TestMPVPersistentLocalIPCCheckpointsEverySecond(t *testing.T) {
	flags, tracker, err := newMPVTracker()
	if err != nil {
		t.Fatal(err)
	}
	defer tracker.close()
	listener, err := listenTestMPV(strings.TrimPrefix(flags[0], "--input-ipc-server="))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	checkpoints := make(chan Progress, 16)
	stop := tracker.start(ctx, func(p Progress) { checkpoints <- p })
	defer stop()
	// The server accepts exactly one connection for the entire session.
	connection := make(chan net.Conn, 1)
	go func() {
		conn, _ := listener.Accept()
		connection <- conn
	}()
	var conn net.Conn
	select {
	case conn = <-connection:
		if conn == nil {
			t.Fatal("IPC accept failed")
		}
	case <-ctx.Done():
		t.Fatal("IPC connection timed out")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := readMPVSubscriptions(conn); err != nil {
		t.Fatal(err)
	}
	for _, position := range []float64{10, 125.75, 126.75} {
		if _, err := fmt.Fprintf(conn, "{\"event\":\"property-change\",\"name\":\"time-pos\",\"data\":%f}\n", position); err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-checkpoints:
			if got.Position != position {
				t.Fatalf("checkpoint = %#v; want %f", got, position)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("no checkpoint within the one-second interval allowance")
		}
	}
}

func TestMPVPersistentConnectionDoesNotLimitLifetimeTraffic(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	_ = server.SetDeadline(time.Now().Add(5 * time.Second))
	done := make(chan error, 1)
	go func() {
		defer server.Close()
		if err := readMPVSubscriptions(server); err != nil {
			done <- err
			return
		}
		// More than 1 MiB in small valid messages must not exhaust the connection.
		message := `{"event":"client-message","data":"` + strings.Repeat("x", 1024) + `"}` + "\n"
		if _, err := io.WriteString(server, strings.Repeat(message, 1100)); err != nil {
			done <- err
			return
		}
		_, err := fmt.Fprintln(server, `{"event":"property-change","name":"time-pos","data":321.5}`)
		done <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var got Progress
	err := observeMPV(ctx, client, func(p Progress) { got = p }, time.Hour)
	if !errors.Is(err, io.EOF) || got.Position != 321.5 {
		t.Fatalf("long-lived connection checkpoint = %#v, %v", got, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
