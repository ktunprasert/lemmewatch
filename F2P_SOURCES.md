# F2P Stream Source Research

Research date: 2026-08-31

Source discussion:
https://www.reddit.com/r/StremioAddons/comments/1un87fe/the_best_http_addons/

WebStreamr support was implemented on 2026-08-31 behind the common playback
provider contract. Flix-Streams Free and PenguPlay remain research candidates.

## Shortlist

### WebStreamrMBG

- Public instance: `https://87d6a6ef6b58-webstreamrmbg.baby-beamup.club`
- Source: `https://github.com/newman2x/WebStreamrMBG`
- No user account or API key required.
- Root manifest works without configuration.
- Configured manifest paths contain URL-encoded JSON and remain stateless.
- Public hostname looks temporary, but project README labels it stable. Keep it
  configurable through `LEMMEWATCH_STREAM_URL` in case deployment changes.
- Stream responses contain addon `/extract/` URLs. These resolve during
  playback rather than exposing permanent origin URLs.
- Returned default streams did not require Stremio `proxyHeaders`.
- Many streams are marked `notWebReady` and `no seek`.
- Some supported hosters require an optional MediaFlow proxy. README lists
  Fastream, FileLions, FileMoon, LuluStream, Mixdrop, Streamtape, and VOE.
- Public instance owns required TMDB credentials; users do not need them.

Current status: first F2P provider. Open source, no authentication, stateless
configuration, and playback URLs are handed to the local player.

### Flix-Streams Free

- Public instance: `https://free.flixnest.app`
- No user account or API key required.
- Manifest declares `configurationRequired: false`.
- Root manifest and stream endpoints work without a configured manifest URL.
- Stable branded hostname presents lower URL-maintenance risk.
- Free sources include HDHub, HDHub4u, Dailymotion, and live TV/sports.
- Responses contain signed Flix proxy URLs, not permanent origin URLs.
- Dailymotion HLS was verified with a plain GET and returned a valid M3U8.
- HDHub results are marked `notWebReady` and include per-stream
  `behaviorHints.proxyHeaders`. One tested upstream returned HTTP 502, including
  when supplied returned headers, so reliability needs broader testing.
- Configured manifests use long opaque path tokens, but defaults do not require
  one.

Current assessment: useful fallback. Full support may require preserving and
forwarding Stremio `behaviorHints.proxyHeaders` to compatible players.

### PenguPlay

- Public instance: `https://pengu.uk`
- Authentication currently required for every user. An unauthenticated stream
  request returned a `You must sign in` placeholder.
- Supports Google OAuth in a browser or a Pengu token obtained through its
  Discord/Telegram flows.
- No documented device-code or CLI OAuth exchange was found.
- Practical CLI integration would accept an existing Pengu token or complete
  configured manifest URL. Automating browser OAuth would depend on
  undocumented callback behavior.
- Auth token is embedded in the configured manifest URL path.
- Token has no visible refresh flow or documented expiration. Invalid or
  revoked tokens require authentication again.

Current assessment: source coverage may be strong, but mandatory account flow
conflicts with low-friction F2P goal.

## Playback Model

These addons do not consistently return permanent final media-host URLs. They
return temporary, signed, resolver, or proxy URLs intended for immediate
playback. Lemmewatch can still pass such URLs to mpv, VLC, or another player as
long as required redirects and request headers are preserved.

Lemmewatch now preserves HTTP `url` entries and selects either its TorBox bundle
or WebStreamr provider. Remaining work for broader HTTP-provider support:

1. Forward `behaviorHints.proxyHeaders` through compatible players.
2. Improve presentation of `notWebReady`, non-seekable, and temporary links.
3. Add Flix or Pengu only through the existing provider contract.

## Security Note

Pengu configured URLs carry bearer credentials in the URL path. Current verbose
HTTP diagnostics omit query strings but include paths. Pengu support must redact
credential-bearing path segments before logging. Temporary signed playback URLs
should receive equivalent treatment even when their secret is not in a query
parameter.

## Recommendation

Continue testing WebStreamr across movies, episodes, qualities, mpv, and VLC.
Revisit Flix-Streams Free as fallback after request-header playback exists.
