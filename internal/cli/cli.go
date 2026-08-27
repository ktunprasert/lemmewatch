package cli

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"lemmewatch/internal/app"
	"lemmewatch/internal/catalog"
	"lemmewatch/internal/httpx"
	"lemmewatch/internal/model"
	"lemmewatch/internal/player"
	"lemmewatch/internal/stremio"
	"lemmewatch/internal/torbox"
)

func New() *cobra.Command {
	verbose := false
	a := configuredApp(&verbose)
	root := &cobra.Command{
		Use: "lemmewatch [QUERY...]", Short: "Find and stream media", SilenceUsage: true, SilenceErrors: true,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return a.Watch(cmd.Context(), strings.Join(args, " "))
		},
	}
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "show sanitized HTTP diagnostics")
	root.AddCommand(watchCommand(a), searchCommand(a), streamsCommand(a), cacheCommand(a), playCommand(a))
	return root
}

func configuredApp(verbose *bool) app.App {
	httpClient := &http.Client{Timeout: 20 * time.Second, Transport: httpx.LoggingTransport{Verbose: verbose, Output: os.Stderr}}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ForceAttemptHTTP2 = false
	transport.TLSClientConfig = &tls.Config{NextProtos: []string{"http/1.1"}}
	transport.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
	torboxHTTP := &http.Client{Timeout: 20 * time.Second, Transport: httpx.LoggingTransport{Base: transport, Verbose: verbose, Output: os.Stderr}}
	playerName, playerArguments := defaultPlayer(runtime.GOOS)
	if configured := os.Getenv("LEMMEWATCH_PLAYER"); configured != "" {
		playerName, playerArguments = configured, nil
	}
	return app.App{
		Catalog: catalog.Client{BaseURL: env("LEMMEWATCH_CATALOG_URL", "https://v3-cinemeta.strem.io"), HTTP: httpClient},
		Streams: stremio.Client{BaseURL: env("LEMMEWATCH_STREAM_URL", "https://torrentio.strem.fun"), HTTP: httpClient},
		TorBox:  torbox.Client{BaseURL: env("TORBOX_API_URL", "https://api.torbox.app/v1/api"), Token: os.Getenv("TORBOX_API_TOKEN"), HTTP: torboxHTTP},
		Player:  player.Player{Executable: playerName, Arguments: playerArguments, Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr},
		In:      os.Stdin, Out: os.Stdout, Err: os.Stderr,
	}
}

func watchCommand(a app.App) *cobra.Command {
	return &cobra.Command{Use: "watch QUERY...", Short: "Find and play a movie", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return a.Watch(cmd.Context(), strings.Join(args, " ")) }}
}

func searchCommand(a app.App) *cobra.Command {
	var kind string
	cmd := &cobra.Command{Use: "search QUERY...", Short: "Search catalog", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if kind != "all" && kind != string(model.Movie) && kind != string(model.Series) {
			return fmt.Errorf("invalid media type %q", kind)
		}
		mediaType := model.MediaType(kind)
		if kind == "all" {
			mediaType = ""
		}
		items, err := a.Search(cmd.Context(), strings.Join(args, " "), mediaType)
		if err != nil {
			return err
		}
		for _, item := range items {
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", item.ID, item.Type, item.Name)
		}
		return nil
	}}
	cmd.Flags().StringVar(&kind, "type", "all", "media type: all, movie, or series")
	return cmd
}

func streamsCommand(a app.App) *cobra.Command {
	return &cobra.Command{Use: "streams IMDB_ID", Short: "List stream candidates", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		items, err := a.LookupStreams(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		for _, s := range items {
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%d\t%dp\t%d\t%d\t%s\n", s.Hash, s.FileIndex, s.Quality, s.Seeders, s.Size, oneLine(s.Title))
		}
		return nil
	}}
}

func cacheCommand(a app.App) *cobra.Command {
	return &cobra.Command{Use: "cache HASH...", Short: "Check TorBox cache", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		cached, err := a.Cache(cmd.Context(), args)
		if err != nil {
			return err
		}
		for _, hash := range args {
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%t\n", hash, cached[hash])
		}
		return nil
	}}
}

func playCommand(a app.App) *cobra.Command {
	var index int
	cmd := &cobra.Command{Use: "play HASH", Short: "Resolve cached torrent and launch player", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if a.TorBox.Token == "" {
			return fmt.Errorf("TORBOX_API_TOKEN is required")
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "Resolving stream through TorBox...")
		u, err := a.TorBox.Resolve(cmd.Context(), args[0], index)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "Launching player...")
		return a.Player.Play(cmd.Context(), u)
	}}
	cmd.Flags().IntVar(&index, "file-index", 0, "video file index")
	return cmd
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func defaultPlayer(goos string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", nil
	case "linux":
		return "xdg-open", nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		return "mpv", nil
	}
}
func oneLine(value string) string {
	return strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(value)
}
