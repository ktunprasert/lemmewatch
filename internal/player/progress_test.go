package player

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"lemmewatch/internal/model"
)

func TestTrackingFlagsSupportWindowsAndUnixPlayerPaths(t *testing.T) {
	for _, executable := range []string{"vlc", `/usr/bin/vlc`, `C:\Program Files\VideoLAN\VLC\vlc.exe`, "mpv", `C:\mpv\MPV.EXE`} {
		t.Run(executable, func(t *testing.T) {
			p := Player{Executable: executable, Arguments: []string{"--fullscreen"}, ResumeSeconds: 123, OnProgress: func(Progress) {}}
			args, tracker := p.prepareTracking()
			if tracker == nil {
				t.Fatal("tracking not configured")
			}
			defer tracker.close()
			want := "--start=123"
			if p.name() == "vlc" {
				want = "--start-time=118"
				if !slices.Contains(args, "--no-one-instance") || !slices.Contains(args, "--http-host=127.0.0.1") {
					t.Fatalf("VLC tracking not isolated: %#v", args)
				}
			}
			if !slices.Contains(args, want) || args[0] != "--fullscreen" {
				t.Fatalf("arguments = %#v", args)
			}
			if !slices.Equal(p.Arguments, []string{"--fullscreen"}) {
				t.Fatal("configured arguments mutated")
			}
		})
	}
	for _, executable := range []string{"xdg-open", "rundll32", "custom-player"} {
		args, tracker := (Player{Executable: executable, ResumeSeconds: 123, OnProgress: func(Progress) {}}).prepareTracking()
		if tracker != nil || len(args) != 0 {
			t.Fatalf("unsupported player %q received tracking flags: %#v", executable, args)
		}
	}
}

func TestVLCProgressHandlesPauseStopAndUnavailableStatus(t *testing.T) {
	for _, test := range []struct {
		name      string
		body      string
		status    int
		want      Progress
		wantError bool
	}{
		{name: "playing", body: `{"time":123,"length":1800,"state":"playing"}`, want: Progress{123, 1800}},
		{name: "paused", body: `{"time":123,"length":1800,"state":"paused"}`, want: Progress{123, 1800}},
		{name: "stopped", body: `{"time":0,"length":0,"state":"stopped"}`, wantError: true},
		{name: "missing time", body: `{"state":"playing"}`, wantError: true},
		{name: "malformed", body: `invalid`, wantError: true},
		{name: "unauthorized", status: http.StatusUnauthorized, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				user, password, ok := r.BasicAuth()
				if !ok || user != "" || password != "session-secret" || r.URL.Path != "/requests/status.json" {
					t.Errorf("incorrect control request")
				}
				if test.status != 0 {
					w.WriteHeader(test.status)
				}
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			got, err := pollVLC(ctx, server.Client(), server.URL+"/requests/status.json", "session-secret")
			if (err != nil) != test.wantError || got != test.want {
				t.Fatalf("progress = %#v, %v", got, err)
			}
		})
	}
}

func TestProgressTrackerCancellationInterruptsBlockedPoll(t *testing.T) {
	started := make(chan struct{})
	tracker := progressTracker{poll: func(ctx context.Context) (Progress, error) {
		close(started)
		<-ctx.Done()
		return Progress{}, ctx.Err()
	}}
	stop := tracker.start(context.Background(), func(Progress) { t.Error("blocked poll saved progress") })
	<-started
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("tracking blocked shutdown")
	}
}

func TestPlayTracksVLCUntilCancelled(t *testing.T) {
	// Run a fake VLC as a real child process to exercise flags, polling, and shutdown.
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(t.TempDir(), "vlc.exe")
	if err := os.WriteFile(child, binary, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LEMMEWATCH_TEST_VLC", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var got Progress
	quiet := false
	p := Player{
		Executable: child, Arguments: []string{"-test.run=^TestVLCProcessHelper$", "--"},
		ResumeSeconds: 125, Verbose: &quiet,
		OnProgress: func(progress Progress) { got = progress; cancel() },
	}
	err = p.Play(ctx, model.Playback{URL: "https://example.invalid/video?token=secret"})
	if err == nil || ctx.Err() != context.Canceled || got != (Progress{123, 1800}) {
		t.Fatalf("play = %v, context = %v, progress = %#v", err, ctx.Err(), got)
	}
}

func TestVLCProcessHelper(t *testing.T) {
	if os.Getenv("LEMMEWATCH_TEST_VLC") != "1" {
		return
	}
	value := func(prefix string) string {
		for _, arg := range os.Args {
			if strings.HasPrefix(arg, prefix) {
				return strings.TrimPrefix(arg, prefix)
			}
		}
		return ""
	}
	if value("--start-time=") != "120" || os.Args[len(os.Args)-1] != "https://example.invalid/video?token=secret" {
		t.Fatal("missing resume flag or media URL")
	}
	server := &http.Server{Addr: "127.0.0.1:" + value("--http-port="), ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, password, _ := r.BasicAuth()
		if password != value("--http-password=") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"time":123,"length":1800,"state":"paused"}`)
	})}
	t.Fatal(server.ListenAndServe())
}
