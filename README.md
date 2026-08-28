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
lemmewatch --query history
lemmewatch watch Dune
lemmewatch search [--type=all|movie|series] Dune
lemmewatch streams tt1160419
lemmewatch cache HASH...
lemmewatch play [--file-index=INDEX] HASH
```

Bare queries matching command names resolve as commands. Use `-q`/`--query` to
force search input, for example `lemmewatch -q history` or
`lemmewatch -q one piece`.

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
`Ctrl-D` and `Ctrl-U` move half a page. Breadcrumbs begin with the active
Movie/Series group and are mirrored in the terminal title.
While filtering, Ctrl-W clears a word and Ctrl-U clears the line. In the torrent
pane, `c` toggles cached/all and `v` cycles quality; quality preference persists
under the XDG config directory. The active Movie/Series tab persists there too.
Uncached playback is not implemented yet.

Playback leaves the browser open. Press `s` to stop a directly managed player,
or navigate back through episodes and titles while it runs. Native `open`,
`xdg-open`, and Windows URL handoff cannot stop the external application.
`lemmewatch history` opens up to 100 recently played top-level IMDb titles in
the browser. History starts as a single root pane without movie/series tabs;
opening titles uses the same season, episode, torrent, and playback flow.
Press `Ctrl-H` from search or History to open the History root. Press `Ctrl-P`
from either root to run a new movie/series search and restore its tabs.
Right/`l` opens the active left item when its child pane is not loaded; only `q`
exits the browser.

At the root, `s` opens a sort-key menu: `a`/`A` sorts title ascending/descending,
`y`/`Y` sorts year ascending/descending, and `d` or `r` restores Cinemeta
relevance. In the torrent pane, `s` offers quality, cache status, name, and
default ranking sorts. `x` stops directly managed playback.

Each list row uses a left-aligned name and right-aligned contextual detail.
Press `m` for pane-specific modes: media year/ID/type, season episode count,
episode air date/ID, and torrent quality/cache/size/seeders/source/filename.
Year, quality, and cache sorting automatically select their matching mode.
Episodes with future air dates use muted text to indicate that they may not be
available yet.
Filter, search, sort, and help use modal overlays. Press `?` for a searchable
keybinding palette; type to filter commands and press Enter to run the selected
binding.
Errors and short status notifications appear as bottom-right toasts and dismiss
automatically without animation.

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

Builds embed `git describe --always --dirty`. Verify an artifact with:

```sh
lemmewatch --version
```
