# Lemmewatch

Local-first CLI/TUI for finding and streaming movies through Stremio-compatible
catalog and stream addons, TorBox, and a local media player.

## Setup

Install pinned Go toolchain and build:

```sh
mise install
mise exec -- go build ./cmd/lemmewatch
```

Set `TORBOX_API_TOKEN` in ignored `.mise.local.toml` for cache and playback
operations. An ignored `.env` file is also loaded at startup; existing process
environment variables take precedence. Optional configuration:

```text
LEMMEWATCH_CATALOG_URL
LEMMEWATCH_STREAM_URL
TORBOX_API_URL
LEMMEWATCH_PLAYER
```

`LEMMEWATCH_PLAYER` overrides URL opening. By default, Lemmewatch uses
`xdg-open` on Linux and `open` on macOS so resolved video URLs open with the
desktop's configured handler. Windows uses its registered URL handler through
`rundll32`. Set it to `mpv`, `vlc`, or another executable to force a specific
player.

## Usage

```text
lemmewatch Dune
lemmewatch watch Dune
lemmewatch search [--type=all|movie|series] Dune
lemmewatch streams tt1160419
lemmewatch cache HASH...
lemmewatch play [--file-index=INDEX] HASH
```

Add `--verbose` to show sanitized HTTP method, host/path, status, duration, and
failure class. Query strings are omitted because TorBox URLs may contain tokens.

Bare query and `watch` run movie and series flows. Series traversal adds season
and episode panes before torrent selection. Selected cached streams resolve
through TorBox and launch the configured player.

For multi-file season packs, addon filename metadata selects the matching TorBox
file before season/episode pattern matching or legacy index fallback.

Interactive watch uses two panes: search results on the left and torrents on the
right. `Tab` switches movie/series results. Enter loads or selects, `h`/`l`
changes focus, `j`/`k` moves, and `/` filters the active pane by name or quality.
While filtering, Ctrl-W clears a word and Ctrl-U clears the line. In the torrent
pane, `c` toggles cached/all and `v` cycles quality; quality preference persists
under the XDG config directory. Uncached playback is not implemented yet.

Temporary TorBox download URLs and API tokens are never printed. `resolve`
command from OCaml prototype is intentionally omitted because printing resolved
URL can expose credentials.

## Verification

```sh
mise exec -- gofmt -w .
mise exec -- go vet ./...
mise exec -- go test -race ./...
mise exec -- go build ./cmd/lemmewatch
mise run build-darwin-arm64
mise run build-windows-amd64
```
