# Lemmewatch

Local-first CLI/TUI for finding and streaming movies through Stremio-compatible
catalog and stream addons, WebStreamr, PenguPlay, or TorBox, and a local media
player.

> [!IMPORTANT]
> Lemmewatch is an unofficial project and is not affiliated with or endorsed by
> TorBox, Stremio, Cinemeta, Torrentio, WebStreamr, or PenguPlay. It does not
> provide media. Users are responsible for complying with applicable laws and
> third-party service terms.

https://github.com/user-attachments/assets/a2cc832c-c799-4c26-8949-418951656481

## Setup

Install pinned Go toolchain and build:

```sh
mise install
mise exec -- go build ./cmd/lemmewatch
```

Set `TORBOX_API_TOKEN` in ignored `.mise.local.toml` to use Torrentio with
TorBox cache and playback. Without a token, WebStreamr is selected automatically
and requires no account or API key. An ignored `.env` file is also loaded at
startup; existing process environment variables take precedence. Optional
configuration:

```text
LEMMEWATCH_CATALOG_URL
LEMMEWATCH_STREAM_URL
LEMMEWATCH_WEBSTREAMR_URL
LEMMEWATCH_PROVIDER
PENGUPLAY_MANIFEST_URL
TORBOX_API_URL
LEMMEWATCH_PLAYER
```

`LEMMEWATCH_PROVIDER` accepts `torbox`, `webstreamr`, or `pengu` and overrides
the saved provider preference. Pengu is available only when
`PENGUPLAY_MANIFEST_URL` contains the configured manifest URL copied after
signing in at `https://pengu.uk`. This URL contains a bearer credential: keep it
in ignored local configuration and never publish it. `LEMMEWATCH_STREAM_URL`
configures Torrentio for TorBox. `LEMMEWATCH_WEBSTREAMR_URL` configures
WebStreamr. Without an override or valid saved preference, Lemmewatch selects
TorBox if a token exists and WebStreamr otherwise; Pengu remains a selectable
fallback.

`LEMMEWATCH_PLAYER` overrides URL opening. By default, Lemmewatch uses
`xdg-open` on Linux and `open` on macOS so resolved video URLs open with the
desktop's configured handler. Windows uses its registered URL handler through
`rundll32`. Set it to `mpv`, `vlc`, or another executable to force a specific
player.

Open `?`, select **Settings**, then press Enter to edit saved defaults under
the user's config directory. Up/Down selects a setting; Left/Right cycles media
type, quality, cached-only filtering, player, and pane detail modes. Enter on
Provider cycles available playback providers. Player accepts a custom
executable. `LEMMEWATCH_PLAYER` takes precedence over the saved player
preference.

Player settings may include arguments, for example `mpv.exe --no-border` or
`"C:\\Program Files\\mpv\\mpv.exe" --no-border`. Commands are parsed into
structured arguments and never run through a shell. Failures are appended to
`$XDG_STATE_HOME/lemmewatch/errors.log` (normally
`~/.local/state/lemmewatch/errors.log`) with URLs redacted.

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

Add `--verbose` or set `DEBUG=1` to show player output plus sanitized HTTP
method, host/path, status, duration, and failure class. Player output is hidden
by default. Query strings are omitted because TorBox URLs may contain tokens.

Bare query and `watch` run movie and series flows. Series traversal adds season
and episode panes before stream selection. TorBox candidates resolve through
TorBox; WebStreamr and Pengu HTTP URLs launch directly in the configured player.

For multi-file season packs, addon filename metadata selects the matching TorBox
file before season/episode pattern matching or legacy index fallback.

WebStreamr and Pengu streams may be temporarily unavailable because their HTTP
sources change independently. Some WebStreamr streams are non-seekable. Tested
Pengu 2Peckle and PixelDrain streams supported seeking and mpv playback. Streams
requiring custom request headers are currently unavailable; player-specific
header forwarding remains future work.

Interactive watch uses adaptive navigation panes. Wide terminals show up to
three latest navigation panes at a `1:1:2`
weight; medium terminals use `1:2`, and narrow terminals show the active pane.
Older ancestors slide off the left while breadcrumbs retain their context.
`Tab` switches movie/series results. Enter loads or selects, `h`/`l`
changes focus, `j`/`k` moves, and `/` filters the active pane by name or quality.
`Ctrl-D` and `Ctrl-U` move half a page. Breadcrumbs begin with the active
Movie/Series group and are mirrored in the terminal title.
`gg` moves to the first item and `G` moves to the last. `Esc` clears an active
pane filter before navigating back.
While filtering, Ctrl-W clears a word and Ctrl-U clears the line. In the torrent
pane, `c` toggles cached/all for TorBox and `v` cycles quality; quality preference persists
under the XDG config directory. The active Movie/Series tab persists there too.
Uncached TorBox playback is not implemented yet. Cache filtering does not apply
to direct WebStreamr or Pengu streams.

Playback leaves the browser open. Press `s` to stop a directly managed player,
or navigate back through episodes and titles while it runs. Native `open`,
`xdg-open`, and Windows URL handoff cannot stop the external application.
`lemmewatch history` opens up to 100 recently played top-level IMDb titles in
the browser. History starts as a single root pane without movie/series tabs;
opening titles uses the same season, episode, torrent, and playback flow.
Press `Ctrl-H` from search or History to open the History root. Press `Ctrl-P`
from either root to run a new movie/series search and restore its tabs.
Episode stream results are cached per provider for the current browser session.
Press `r` on an episode or its stream pane to refresh provider results.
Right/`l` opens the active left item when its child pane is not loaded; only `q`
exits the browser.

At the root, `s` opens a sort-key menu: `a`/`A` sorts title ascending/descending,
`y`/`Y` sorts year ascending/descending, and `d` or `r` restores Cinemeta
relevance. In the stream pane, `s` offers quality, cache status, name, and
default ranking sorts. `x` stops directly managed playback.

Each list row uses a left-aligned name and right-aligned contextual detail.
Press `m` for pane-specific modes: media year/rating/ID/type, season episode count,
episode air date/rating/ID, and stream quality/cache/size/seeders/source/filename.
Year, quality, and cache sorting automatically select their matching mode.
Episodes with future air dates use muted text to indicate that they may not be
available yet.
Filter, search, sort, and help use modal overlays. Press `?` for a searchable
keybinding palette; type to filter commands and press Enter to run the selected
binding.
Errors and short status notifications appear as bottom-right toasts and dismiss
automatically without animation.

Temporary playback URLs and API tokens are never printed. `cache HASH...` and
`play HASH` remain TorBox-specific diagnostics. `resolve`
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

Public releases use date-based versions such as `v2026.8.1`. Create and push
the next `vYYYY.M.N` tag to build all supported targets and publish a GitHub
Release with generated notes. Release binaries report the tag through
`--version`.

```sh
git tag v2026.8.1
git push origin v2026.8.1
```

The Windows build produces `lemmewatch.exe` and
`lemmewatch-launcher.exe`. Run `lemmewatch.exe` directly from a terminal or
place it on `PATH`. Explorer and shortcut users can open the sibling GUI
launcher, which creates `lemmewatch.exe` in a new console hosted by the user's
configured Windows default terminal. No shell or terminal executable is
hardcoded. Opening the launcher without arguments starts the interactive
dashboard. Enter searches, Ctrl-H opens history, and Esc exits.
Tab switches the initial Movie/Series result pane and saves that preference.
