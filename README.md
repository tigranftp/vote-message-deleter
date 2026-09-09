# Telegram Vote Moderation Bot

Telegram Vote Moderation Bot is a planned Go service for moderating Telegram
group chats and supergroups using message reactions. It observes
non-anonymous reaction changes, maintains in-memory 👍 and 👎 totals, and
applies moderation policy when those totals change.

The project is currently in early development. Its first moderation decision
rule, a testable Telegram Bot API adapter, and an in-memory application handler
are implemented. The command-line entry point supports real Telegram long
polling with a safe-by-default dry-run mode.

## Design

The planned package layout keeps Telegram API concerns separate from moderation
policy so that moderation rules can be tested without making network requests:

```text
cmd/bot/             application entry point
internal/app/        update orchestration and in-memory vote tracking
internal/config/     configuration loading and validation
internal/telegram/   Telegram HTTP transport, DTOs, and reaction normalization
internal/moderation/ stateless moderation policy
internal/storage/    persistence abstractions and implementations
```

Moderation thresholds will be supplied through configuration rather than
hard-coded in the application.

The Telegram adapter uses the Go standard library, performs one long-poll
request at a time, and supports a configurable base URL for isolated HTTP
tests. The application runner repeats those bounded requests until shutdown.

## Non-anonymous reactions and preservation

The bot subscribes to Telegram `message` and `message_reaction` updates. New
message text and media captions are retained in a bounded in-memory cache for
up to 24 hours. Each reaction update contains one actor's old and new reaction
sets. The application converts that change to a 👍/👎 delta and applies it to
in-memory totals for the affected chat and message:

```text
message -> cache text or caption
message_reaction
    -> compare old_reaction with new_reaction
    -> update in-memory 👍 and 👎 totals
    -> update the in-memory list of current dislikers
    -> apply moderation.Decide
    -> publish a protected spoiler copy
    -> after publication succeeds, call deleteMessage
```

Unknown reactions are ignored. Repeated Telegram updates with the same
`update_id` do not apply their vote delta twice. No usernames or other personal
profile fields are logged.

Vote totals are not persisted. Restarting the process clears all observed
counts, and Telegram provides no absolute total in a non-anonymous
`message_reaction` update. For a reliable first test, start the bot before users
react and use a new message after startup. Existing reactions from before the
process started cannot be reconstructed accurately.

Only text and media captions are preserved in the current version; media files
themselves are not copied. Messages without text or a caption, including stickers,
are deleted after a notice containing “Сообщение без текста или подписи” is sent.
Their metadata is cached just like that of textual messages.
If the original message was not observed by
the running process, the bot logs `SKIP_DELETE` and leaves the original message
untouched. If spoiler publication fails, deletion is not attempted. If spoiler
publication succeeds but deletion fails, the runner retries only the deletion
and does not publish a duplicate spoiler.

## Requirements

- Go 1.24 or later

No external Go dependencies are currently used.

## Configuration

The application reads configuration directly from its process environment.
`.env.example` lists the supported names, but `.env` files are not loaded
automatically. Never commit `.env` or a real Telegram bot token.

Supported variables:

- `TELEGRAM_BOT_TOKEN` — required; no default
- `MODERATION_DISLIKE_THRESHOLD` — optional; defaults to `3`
- `MODERATION_LIKE_THRESHOLD` — optional; defaults to `3`; reserved for future
  positive-vote policy and does not currently cause deletion
- `TELEGRAM_POLL_TIMEOUT` — optional positive integer in seconds; defaults to
  `30`
- `DRY_RUN` — optional boolean; defaults to `true`

Configuration is validated before polling starts. The bot token is never
included in application logs.

## First local run

The bot must be an administrator in the Telegram group to receive the required
non-anonymous reaction updates and to delete messages when live deletion is
enabled.
Start the first test in dry-run mode so the complete moderation flow runs
without sending `deleteMessage`.

From Windows Command Prompt (`cmd.exe`), run:

```bat
set TELEGRAM_BOT_TOKEN=<YOUR_BOT_TOKEN>
set MODERATION_DISLIKE_THRESHOLD=3
set TELEGRAM_POLL_TIMEOUT=30
set DRY_RUN=true
go run ./cmd/bot
```

Startup logs report dry-run status, the dislike threshold, and long-polling
status without printing the token. A non-anonymous reaction update produces a
compact entry such as:

```text
moderation decision: KEEP chat_id=-100123 message_id=77 likes=4 dislikes=1
moderation decision: WOULD_DELETE chat_id=-100123 message_id=77 likes=4 dislikes=3
```

After validating behavior, setting `DRY_RUN=false` enables live moderation. On
a `DeleteMessage` decision, the bot first sends the cached text or caption as a
protected spoiler with the original message ID, original publication time, and
the observed negative-reaction count. It lists the users whose 👎 reactions are
still active when the threshold is reached. Usernames are shown as clickable
`@username` tags; users without public usernames get clickable display names
linked through their Telegram user IDs. The notice also includes the sender's
display name and username. For forwarded messages, the bot shows the origin
reported by Telegram. Public users and chats link to their profile, and public
channel posts link directly to the original post. Hidden and private origins
remain plain text. For a forwarded post from a public channel, the notice also
includes the direct `https://t.me/<channel>/<message_id>` URL on a separate
`Original` line. Publication time uses the forwarded origin's date when
available; otherwise it uses the message date. The time is rendered in the
process's local time zone with its numeric UTC offset.

The bot calls Telegram's `deleteMessage` only after `sendMessage` succeeds.
Dry-run mode neither publishes the spoiler nor deletes the original.

The current implementation depends on Telegram `message_reaction` updates for
non-anonymous reactions. The bot must be an administrator in the chat and must
remain running while the reactions being counted are added, removed, or
changed.

## Development

Run the standard checks from the repository root:

```sh
gofmt -w .
go test ./...
go vet ./...
```

With the required environment configured, start the bot with:

```sh
go run ./cmd/bot
```
