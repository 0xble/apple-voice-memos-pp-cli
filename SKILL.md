---
name: apple-voice-memos
description: Use apple-voice-memos-pp-cli to refresh, list, search, export, and extract embedded transcripts from Apple Voice Memos on macOS.
tags: [apple, voice-memos, macos, transcription, cli, printing-press]
---

# Apple Voice Memos CLI

Use this skill when the user asks to work with Apple Voice Memos recordings on macOS.

## Prerequisite

The `apple-voice-memos-pp-cli` binary must be on `PATH`. Verify access before handling private recording data:

```bash
apple-voice-memos-pp-cli doctor --agent
```

## Safe defaults

- Reads local macOS Voice Memos data only.
- Makes no application-level network requests.
- Opens Apple’s database read-only and query-only.
- Never modifies or deletes recordings.
- `recent` refreshes through `voicememod` by default. Use `--cached` only when stale local data is explicitly acceptable.
- `list` is cached by default. Use `list --fresh` when current iCloud state matters.
- The sync fallback launches Voice Memos hidden and terminates only the process instance the CLI launched.
- `export` copies one selected `.m4a` to a destination directory with private file permissions.
- Prefer `--agent` for machine-readable output.

## Common commands

```bash
apple-voice-memos-pp-cli doctor --agent
apple-voice-memos-pp-cli sync --agent
apple-voice-memos-pp-cli recent --limit 10 --agent
apple-voice-memos-pp-cli recent --cached --limit 10 --agent
apple-voice-memos-pp-cli list --fresh --search "keyword" --agent
apple-voice-memos-pp-cli transcript <id> --agent
apple-voice-memos-pp-cli export <id> --out ~/Downloads --agent
```

## Operational guidance

- When the user says “latest” or “recent,” do not add `--cached`.
- If refresh fails, report the failure rather than silently presenting cached records as current.
- Do not expose titles, transcripts, UUIDs, or paths beyond what the user requested.
- Do not attach a database or recording to bug reports.
- `transcript` extracts Apple’s embedded `.m4a` `tsrp` transcript. If it is absent, report that honestly. Use another STT tool only when the user asks for transcription.
