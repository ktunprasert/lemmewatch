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
LEMMEWATCH_PENGUPLAY_MANIFEST_URL
TORBOX_API_URL
LEMMEWATCH_PLAYER
```

`LEMMEWATCH_PROVIDER` accepts `torbox`, `webstreamr`, or `pengu` and overrides
the saved provider preference. Pengu is available only when
`LEMMEWATCH_PENGUPLAY_MANIFEST_URL` contains the configured manifest URL copied
after signing in at `https://pengu.uk`. This URL contains a bearer credential:
keep it in ignored local configuration and never publish it.
`LEMMEWATCH_STREAM_URL` configures Torrentio for TorBox.
`LEMMEWATCH_WEBSTREAMR_URL` configures WebStreamr. Without an override or valid
saved preference, Lemmewatch selects TorBox if a token exists and WebStreamr
otherwise; Pengu remains a selectable fallback.

`LEMMEWATCH_PLAYER` overrides URL opening. By default, Lemmewatch uses
`xdg-open` on Linux and `open` on macOS so resolved video URLs open with the
desktop's configured handler. Windows uses its registered URL handler through
`rundll32`. Set it to `mpv`, `vlc`, or another executable to force a specific
player.

Press `;`, or open `?` and select **Settings**, to edit saved defaults under the
user's config directory. Press `;` again or Esc to close. Up/Down selects a
setting; Left/Right cycles media type, quality, cached-only filtering, player,
and pane detail modes. Enter on Provider cycles available playback providers.
Player accepts a custom executable. `LEMMEWATCH_PLAYER` takes precedence over
the saved player preference.

**Audio languages**, **Subtitle languages**, **Playback speed**, **Autoplay next
episode**, and **Remember playback position** are also saved in Settings.
Left/Right cycles languages, common speeds, or toggles. Enter edits an
ordered, comma-separated language list, for example `ja,en` for Japanese first,
then English. Language names, ISO two/three-letter codes, and region tags such as
`pt-BR` are accepted. Clear the field with Ctrl-U and save to use the default.
Speed accepts 0.25–4, including custom values such as `1.35`; clearing it leaves
the player's default speed in effect. Changes apply to the next playback.

Default stream ranking prefers audio languages in order, then subtitle languages,
while preserving title/episode matching priority and existing quality/cache filters.
Unknown-language releases remain available ahead of known nonmatches; equally
matched releases retain their original ranking. Explicit stream sorts override
language ranking. Changing language settings immediately reranks loaded streams.
Stream info (`i`) shows the available release language hints.

[Torrentio language flags](https://github.com/TheBeastLT/torrentio-scraper/blob/master/addon/lib/languages.js)
and release names are hints, not verified track lists.
English-only and some Japanese flags may be omitted; `MULTI`, `DUAL`, and the
ambiguous Indian flag do not establish a specific language. Explicit `Audio:`
and `Subtitles:`/`Subs:` sections are recognized separately. Generic flags help
audio ranking but never claim a subtitle track. TorBox's cache API supplies file
names rather than track languages; its separate Pro streaming API exposes audio
and subtitle metadata after creating a stream ([TorBox API docs](https://api-docs.torbox.app/)). This app uses direct file playback
and release hints, without requiring that Pro API.

Directly selected **mpv** and **VLC** receive ordered audio/subtitle language
preferences and playback speed. They select the first available preferred track,
then fall back to normal track selection. VLC uses the base language for region
tags. Preferences select tracks available to the player; they do not download
external subtitles or add missing audio tracks. System URL handlers and other
custom players receive the URL only and use their own playback preferences.

When VLC or mpv is selected directly and **Remember playback position** is on,
browser playback automatically saves the position and resumes next time without
a prompt. This setting defaults on. Turning it off skips loading and saving
checkpoints without deleting existing ones; turning it back on can resume a
previous checkpoint. Resume uses CLI flags
(`--start-time` for VLC, `--start` for mpv); local control interfaces are enabled automatically to read
the position. VLC uses a dedicated instance with a password-protected loopback
HTTP interface, polled roughly every five seconds, and resumes five seconds
before its checkpoint. mpv keeps a private local socket on Linux/macOS or a named
pipe on Windows open and subscribes to position
changes. It saves checkpoints every second and flushes the latest received
position when playback stops or the connection closes. mpv resumes at the saved
second without a rewind; unchanged positions skip writes. No player setup is
required. URL-handler launches and the diagnostic `play HASH` command do not
track position.

Checkpoints follow the movie or episode across refreshed stream URLs and are
independent of watched state. Switching to a different cut may shift the scene;
non-seekable streams may not resume. Playing into the last 15 seconds (or last
5% for short clips) clears the checkpoint. To restart an unfinished title, seek
to the beginning in the player. If tracking is unavailable, playback continues
and the previous checkpoint remains.

Player settings may include arguments, for example `mpv.exe --no-border` or
`"C:\\Program Files\\mpv\\mpv.exe" --no-border`. Commands are parsed into
structured arguments and never run through a shell. Failures are appended to
`$XDG_STATE_HOME/lemmewatch/errors.log` (normally
`~/.local/state/lemmewatch/errors.log`) with URLs redacted.

## Storage

Saved settings remain in `lemmewatch/preferences.json` under the platform's user
config directory. Copy that file to move only settings to another machine. It
contains the saved TorBox token in plain text and is created with user-only file
permissions, so transfer it securely. Environment variables, `.env`, logs,
history, caches, installed players, and OS URL associations are not included.

History, watched state, and resume positions live in `lemmewatch/history.db`
under the user config directory. On first launch, an existing `history.json` is
imported atomically and renamed to `history.json.migrated`. Lemmewatch holds this bbolt database for
the command's lifetime. A second session waits up to 250 milliseconds, then
exits with a message asking the user to close the active session. Help and
version output do not open storage.

Rebuildable data lives in `lemmewatch/cache.db` under the platform's user cache
directory. This directory normally survives reboots but may be removed by the
OS, cleanup tools, or the user. Cache failures do not prevent searches or
playback. Expired entries are deleted lazily and database pages are reused.

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
three navigation panes; medium terminals show two, and narrow terminals show
the active pane. Two panes split the width equally. Three panes use a
10/30/60 split: a collapsed left parent and a wider right pane. These ratios
stay fixed regardless of content, focus, or info visibility. Pane titles and
selection counts sit in the borders. Settings, key help, search/filter,
sort/mode menus, and input popups use border titles too.
To customize the split, open Settings (`;`), select **Two-pane sizes** or
**Three-pane sizes**, and press Enter. Enter left-to-right relative sizes such
as `1:1` or `1:2:3` (each value 1–1000). Changes apply immediately and persist
in `preferences.json` under `"pane_sizes": {"2": [50, 50], "3": [10, 30, 60]}`.
Missing or invalid ratios use the defaults. Small panes retain an 18-column
minimum, so extreme ratios are adjusted to fit the terminal.
Older ancestors slide off the left while breadcrumbs retain their context.
Press `i` to toggle selected-item info at the bottom of the focused pane.
Movies and series show a concise synopsis, rating, genres, runtime, status,
country, and a short cast/crew list from Cinemeta. Seasons show only episode and
watched counts, release dates, and the show name. Episodes show one synopsis,
release date, rating, and watched state. Streams show release title, quality,
size, seeders, availability, source, filename, and language hints, with compact
TorBox download progress, pack file count, and file format when available.

Info panels omit duplicate fields, internal IDs/hashes, behavior flags, raw file
lists, and URL placeholders. Extra provider metadata is still retained for useful
fields; credentials, request headers, and playback URLs are excluded. Inspecting
info never queues a torrent or creates a Pro stream.
Requests run in the background and cancel when selection changes. Cached/basic
info stays visible on failure; close and reopen `i` to retry. Full Cinemeta details
are cached for 30 days, while TorBox account details remain session-only. Show
refresh (`r`/`F5` in seasons/episodes) updates the full metadata cache too.

Rating detail mode (`m`, then `r`) loads missing Cinemeta ratings for visible
movie/series rows, including History, even with info closed. Ratings appear as
requests complete. Episode rows use their episode rating when provided; otherwise
the show rating appears with a `show` suffix. `--` means no rating is available.

External text is normalized before rendering: emoji, wide decorative symbols,
terminal escapes, invisible formatting controls, and repeated whitespace are
removed. Ordinary international text is measured and clipped by terminal-cell
width; pane padding and alignment remain intact. Each pane type remembers its toggle for
the session. Info follows selection, uses about one-third of the pane height,
and wraps long text; `Alt-j`/`Alt-k` scroll overflowing details. Toggling info
keeps pane widths stable. The footer shows contextual shortcuts; `?` opens
the full searchable key list.
`Tab` switches movie/series results. Enter loads or selects, `h`/`l`
changes focus, `j`/`k` moves, and `/` filters the active pane by name or quality.
`Ctrl-D` and `Ctrl-U` move half a page. Breadcrumbs begin with the active
Movie/Series group and are mirrored in the terminal title.
`gg` moves to the first item and `G` moves to the last. `Esc` clears an active
pane filter before navigating back.
While filtering, Ctrl-W clears a word and Ctrl-U clears the line. In the torrent
pane, `c` toggles cached/all for TorBox and `v` cycles quality; quality preference persists
under the XDG config directory. The active Movie/Series tab persists there too.
Selecting an uncached TorBox stream queues the torrent (reusing an existing
copy on TorBox without re-adding), shows a spinning progress toast while the
download runs (up to 30 minutes), and starts playback when ready. Cache
filtering does not apply
to direct WebStreamr or Pengu streams.

Playback leaves the browser open. Press `x` to stop a directly managed player,
or navigate back through episodes and titles while it runs. Native `open`,
`xdg-open`, and Windows URL handoff cannot stop the external application.
When **Autoplay next episode** is enabled with directly selected mpv or VLC,
Lemmewatch starts looking up the next aired episode and its ranked streams 60
seconds before the measured end of playback. This prefetch does not change the
visible episode, resolve a final playback URL, or queue an uncached torrent.
At the completion threshold (the final 15 seconds, or 5% for short clips), the
browser closes the current managed player and immediately starts the best
playable stream matching the current quality, availability, language, and sort
preferences. Autoplay crosses season boundaries and stops on future or
missing episodes, lookup failures, missing matching streams, manual stops, or
playback closed before the completion threshold. System URL handlers cannot
report reliable progress, so they never autoplay.
`lemmewatch history` opens up to 100 recently played top-level IMDb titles in
the browser. History starts as a single root pane without movie/series tabs;
opening titles uses the same season, episode, torrent, and playback flow.
When History loads, `+` in its status column means the unexpired series
metadata cache may contain a newer aired episode than the highest watched
episode. This indicator is intentionally approximate and never triggers a
network request.
Press `Ctrl-H` from search or History to open the History root. Press `Ctrl-P`
from either root to run a new movie/series search and restore its tabs.
Press `w` on a root title to add it to or remove it from history. In History,
press `d` to remove the selected title. In season and episode panes, `W` toggles
every row through the selected row as watched or unwatched; `w` still toggles
only the selected row.
Search results are cached for 24 hours. Complete series season and episode
metadata is cached for 30 days. Stable Torrentio candidates are cached for 24
hours, while TorBox availability is checked when those candidates load into a
browser session. Temporary WebStreamr and Pengu URLs remain session-only and are
never written to disk. Press `r` or `F5` inside a season or episode list to
fetch fresh metadata for the whole show, including new seasons and episodes.
The browser preserves the selected season/episode when it still exists and
keeps the last good panes if refresh fails. In the stream pane, `r`/`F5`
refreshes stream candidates and availability instead.
Refresh is disabled while the root Movies/Series
or History pane is focused. Failed refreshes leave the last good
disk cache available for the next load.
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
