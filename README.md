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
operations. Optional configuration:

```text
LEMMEWATCH_CATALOG_URL
LEMMEWATCH_STREAM_URL
TORBOX_API_URL
LEMMEWATCH_PLAYER
```

`LEMMEWATCH_PLAYER` defaults to `mpv`.

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

Bare query and `watch` run movie flow: search, select movie, query stream addon,
filter to cached candidates, select stream, resolve through TorBox, and launch
player. Series playback is not implemented yet.

Interactive watch uses two panes: search results on the left and cached torrents
on the right. Enter loads or selects, `h`/`l` changes focus, `j`/`k` moves,
Page Up/Down pages, and `/` filters the active pane. Escape clears a filter or
returns to the left pane.

Temporary TorBox download URLs and API tokens are never printed. `resolve`
command from OCaml prototype is intentionally omitted because printing resolved
URL can expose credentials.

## Verification

```sh
mise exec -- gofmt -w .
mise exec -- go vet ./...
mise exec -- go test -race ./...
mise exec -- go build ./cmd/lemmewatch
```
