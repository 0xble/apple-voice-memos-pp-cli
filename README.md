# apple-voice-memos-pp-cli

Local, read-only CLI for Apple Voice Memos on macOS, built in the Printing Press style.

It reads the modern iCloud-synced Voice Memos store:

```text
~/Library/Group Containers/group.com.apple.VoiceMemos.shared/Recordings/CloudRecordings.db
~/Library/Group Containers/group.com.apple.VoiceMemos.shared/Recordings/*.m4a
```

No network calls. No Apple database writes. The only write command is `export`, which copies a selected `.m4a` to an output directory.

## Install locally

```bash
cd /Users/brianle/Projects/apple-voice-memos-pp-cli
go build -o bin/apple-voice-memos-pp-cli .
```

Optional:

```bash
cp bin/apple-voice-memos-pp-cli ~/.local/bin/
```

## Commands

```bash
apple-voice-memos-pp-cli doctor --json
apple-voice-memos-pp-cli list --limit 20
apple-voice-memos-pp-cli list --search "meeting" --json
apple-voice-memos-pp-cli recent --limit 10
apple-voice-memos-pp-cli transcript <id|uuid|filename>
apple-voice-memos-pp-cli transcript <id> --raw
apple-voice-memos-pp-cli export <id|uuid|filename> --out ~/Downloads
apple-voice-memos-pp-cli agent-context --pretty
apple-voice-memos-pp-cli which "summarize memo" --json
```

## Transcript extraction

`transcript` extracts Apple's embedded transcript from the `.m4a` `tsrp` atom. It does not re-transcribe audio. If a memo has no `tsrp` atom, the command fails honestly with `tsrp transcript atom not found`.

## Flags

Global flags:

```text
--db PATH              Override CloudRecordings.db path
--recordings-dir DIR   Override recordings directory
--json                 Emit JSON
--agent                JSON, no color, non-interactive defaults
--no-color             Disable color output
```

## Printing Press status

This is a local hand-built Printing Press-style CLI because the public Printing Press catalog currently has no Apple Voice Memos entry and `cli-printing-press generate` targets HTTP/API specs rather than local macOS SQLite stores. The repo includes `.printing-press.json`, CLI naming, `agent-context`, `which`, JSON/agent modes, and verification-friendly commands so it can be packaged or promoted later.
