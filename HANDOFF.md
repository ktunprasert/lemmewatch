# Lemmewatch Handoff

Local-first CLI/TUI for finding and streaming media. This repository is an
OCaml prototype; next implementation may use another language.

## Intended User Flow

```text
lemmewatch Dune
-> search Cinemeta
-> select movie or show
-> select season and episode when needed
-> query Torrentio through the Stremio addon protocol
-> normalize, deduplicate, filter, and rank streams
-> select stream
-> resolve torrent through TorBox
-> launch mpv, VLC, or another configured player
```

Lemmewatch is a Stremio protocol client, not a scraper. Remote addon requests
go directly to configurable addon endpoints. Local Stremio Service support was
planned as an optional resolver, not as a source of account, profile, or addon
state.

## Implemented

- Cinemeta movie and series search, including concurrent combined search.
- Torrentio-compatible stream lookup by IMDb ID.
- Torrent info-hash and `fileIdx` parsing and validation.
- TorBox cache checks, cached-only torrent creation, file lookup, and temporary
  download URL resolution.
- Filtering TorBox auxiliary files so Torrentio video indexes map correctly.
- TorBox `append_name=true` download URLs for player filename detection.
- Structured player argv launching without shell interpolation.
- Player error sanitization so resolved URLs and API tokens are not included in
  launch errors.
- Notty Community selector with arrows, `j`/`k`, Page Up/Down, Enter,
  `q`/Escape, scrolling, resize handling, and terminal release.
- Interactive movie flow: search, movie selection, stream selection, TorBox
  resolution, and player launch.
- Bare query configured as default Cmdliner term, intended to make
  `lemmewatch Dune` equivalent to `lemmewatch watch Dune`.
- CLI commands for `search`, `streams`, `cache`, `resolve`, `play`, and `watch`.
- Alcotest suites for Cinemeta, stream parsing, TorBox, player, and selector
  behavior.
- Mise/opam toolchain lock.

## Current Problems

- CLI help is reported broken. Reproduce and fix `lemmewatch --help`,
  `lemmewatch help`, and subcommand help before further feature work. Likely
  interaction between Cmdliner's group default term and positional bare query.
- `dune exec lemmewatch -- watch Dune` launched the app in a separate process
  group without giving it terminal foreground ownership. Notty then triggered
  `SIGTTOU` and silently stopped. An uncommitted fix in `lib/select_ui.ml`,
  `lib/terminal_stubs.c`, and `lib/dune` temporarily claims foreground terminal
  ownership and restores it after selection. It builds and tests, but needs
  real-terminal verification and review.
- Network operations have no explicit timeout and no visible loading state.
  A slow request therefore looks like a frozen application.
- Interactive watch supports movies only. Search can return series, but season
  and episode metadata/selection are not implemented.
- Stream normalization, deduplication, and meaningful ranking are incomplete.
- `resolve` prints a temporary TorBox URL. Since that URL can contain the API
  token, remove or explicitly redesign this diagnostic command before treating
  token-redaction guarantees as complete.
- `HANDOFF.md` was previously stale and claimed only a scaffold existed.

## Existing CLI

```text
lemmewatch QUERY...
lemmewatch watch QUERY...
lemmewatch search [--type=all|movie|series] QUERY...
lemmewatch streams IMDB_ID
lemmewatch cache HASH...
lemmewatch resolve [--file-index=INDEX] HASH
lemmewatch play [--file-index=INDEX] HASH
```

The first two forms are intended to run the interactive movie pipeline.

## Configuration

- `TORBOX_API_TOKEN`: required for cache, resolve, play, and watch operations.
- `LEMMEWATCH_CATALOG_URL`: Cinemeta-compatible base URL override.
- `LEMMEWATCH_STREAM_URL`: Torrentio/Stremio-compatible stream addon override.
- `TORBOX_API_URL`: TorBox API base URL override.
- `LEMMEWATCH_PLAYER`: player executable; defaults to `mpv`.
- `LEMMEWATCH_STREMIO_SERVICE_URL`: planned but not implemented.

Secrets may be stored in ignored `.mise.local.toml`. Never commit or print that
file. Never log resolved TorBox URLs because they may contain the API token.

## Architecture Boundaries

- `lib/cinemeta.ml`: catalog search and response decoding.
- `lib/stremio_stream.ml`: stream-addon request and response normalization.
- `lib/torbox.ml`: cache checks, torrent creation, file mapping, and URL
  resolution.
- `lib/player.ml`: safe player process execution.
- `lib/select_ui.ml`: terminal selector state, rendering, and input.
- `bin/main.ml`: Cmdliner definitions and orchestration.

Keep adapters narrow enough to replace Cinemeta, Torrentio, or TorBox without
building a general plugin framework prematurely.

## Requirements For Rewrite

- Bare query invocation and explicit subcommands must coexist with reliable
  root and subcommand help.
- Show progress before every network operation and enforce request timeouts.
- Handle terminal foreground ownership, signals, resize, cancellation, and
  restoration under both direct execution and development launchers.
- Support movie and series flows, including season and episode selection.
- Query one or more Stremio stream addons concurrently, isolate failures, and
  apply per-addon timeouts.
- Normalize, deduplicate, filter, and rank candidates using quality, size,
  seeders, and cache status where available.
- Preserve info hash, `fileIdx`, title, filename, seeders, size, and quality.
- Filter resolver file lists to video files before applying addon video index.
- Keep API tokens and resolved URLs out of output, logs, and errors.
- Launch players with structured argv, never shell interpolation.
- Keep startup fast, memory use low, and architecture straightforward.

## Suggested Rewrite Milestones

1. Build CLI parser with tested bare-query and help behavior.
2. Add HTTP client, timeout policy, progress UI, and Cinemeta search.
3. Add robust keyboard selector and terminal lifecycle tests.
4. Add Torrentio lookup, normalization, deduplication, and ranking.
5. Add TorBox cache/create/resolve flow with secret-safe diagnostics.
6. Add mpv/VLC launching and end-to-end movie flow.
7. Add series metadata, season selection, and episode selection.
8. Add concurrent configurable stream addons with isolated failures.
9. Add XDG watch history and continue-watching flow.
10. Research optional local Stremio Service resolution.

## OCaml Prototype Commands

```sh
opam exec -- dune build @all @runtest
opam exec -- dune exec lemmewatch -- watch Dune
```

Project currently uses OCaml 5.3.0, Dune 3.19.0, Eio, Cohttp Eio, TLS Eio,
Yojson, Notty Community, Cmdliner, and Alcotest.

Before distributing with public Cinemeta or Torrentio endpoints as defaults,
confirm third-party client usage is acceptable to their maintainers.
