---
name: pp-apple-voice-memos
description: Use apple-voice-memos-pp-cli to list, search, export, and extract embedded transcripts from Apple Voice Memos on macOS.
tags: [apple, voice-memos, macos, transcription, cli, printing-press]
---

# Apple Voice Memos CLI

Use this skill when Brian asks to work with Apple Voice Memos recordings on macOS.

## Tool

```bash
/Users/brianle/Projects/apple-voice-memos-pp-cli/bin/apple-voice-memos-pp-cli
```

If installed on PATH:

```bash
apple-voice-memos-pp-cli
```

## Safe defaults

- Reads local macOS Voice Memos data only.
- Does not call a network API.
- Does not modify Apple's database.
- `export` copies a selected `.m4a` to an output directory.
- Prefer `--agent` for JSON and stable scripting.

## Common commands

```bash
apple-voice-memos-pp-cli doctor --agent
apple-voice-memos-pp-cli recent --limit 10 --agent
apple-voice-memos-pp-cli list --search "keyword" --agent
apple-voice-memos-pp-cli transcript <id> --agent
apple-voice-memos-pp-cli export <id> --out ~/Downloads --agent
```

## Notes

`transcript` extracts Apple's embedded transcript from the `.m4a` `tsrp` atom. If Apple has not generated a transcript for a memo, the command reports that honestly. Use another STT tool only when embedded transcripts are absent.
