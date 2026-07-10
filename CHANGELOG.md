# Changelog

All notable changes to this project will be documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and releases use [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0] - 2026-07-10

### Added

- Read-only listing and searching of Apple Voice Memos metadata
- Fresh synchronization through `voicememod` with a hidden-app fallback
- Embedded Apple transcript extraction from recording-media `tsrp` atoms
- Private-permission, atomic audio export
- JSON and agent-oriented output modes
- Schema compatibility checks and synthetic fixtures
- GitHub Actions CI and GoReleaser release packaging

### Fixed

- Handle cloud-only recordings whose local `ZPATH` is null
- Explain when transcript or export media is not downloaded locally
- Avoid probing every recording file during a single-memo lookup
- Respect cancellation while finding and stopping a CLI-launched Voice Memos process
- Preserve existing exports and remove temporary files when a copy fails

[Unreleased]: https://github.com/0xble/apple-voice-memos-pp-cli/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/0xble/apple-voice-memos-pp-cli/releases/tag/v0.1.0
