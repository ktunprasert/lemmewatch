# Milestones

## Current: Windows Player Process Output

- [x] Redirect player stdout/stderr to `NUL` in quiet mode.
- [x] Prevent quiet Windows players from attaching to host console with
  `CREATE_NO_WINDOW` and `HideWindow`.
- [x] Preserve normal process output with `--verbose` or `DEBUG=1`.
- [ ] Verify direct CLI and TUI playback on a Windows host.
- Add Windows-focused regression coverage where practical.

## Queued: Reliable Series Episode Metadata

- Add an episode metadata fallback or replacement for stale Cinemeta records.
- Preserve IMDb IDs expected by Torrentio-compatible stream addons.
- Define deduplication and numbering behavior when providers disagree.
- Add regression coverage for `tt30971467`, Monogatari Series: Off & Monster
  Season.

Evidence captured 2026-08-28:

- Cinemeta returns one video: `tt30971467:1:1`.
- IMDb's official daily `title.episode.tsv.gz` dataset contains numbered
  episodes 1-15 plus two child records without season/episode numbers.
- Expected user-visible catalog was 14 episodes; provider reconciliation must
  avoid blindly exposing stale, duplicate, unknown, or future records.
