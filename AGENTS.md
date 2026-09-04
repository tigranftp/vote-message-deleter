# Telegram Vote Moderation Bot

## Project goal

This repository contains a Telegram group moderation bot written in Go.

The bot will:

- run inside Telegram group chats and supergroups;
- observe non-anonymous reaction changes reported for messages;
- maintain positive and negative vote totals in memory;
- delete a message when configured moderation conditions are met;
- support per-chat moderation settings in the future.

Typical reactions:

- 👍 = positive vote
- 👎 = negative vote

Do not hard-code moderation thresholds. Keep moderation policy separate
from Telegram API integration.

## Non-anonymous reaction architecture

The application uses Telegram `message_reaction` updates for non-anonymous
reactions. Each update's `old_reaction` and `new_reaction` values are converted
to a vote delta and applied to in-memory totals for the affected chat and
message.

Vote totals must not be persisted. They reset when the process restarts.
Because non-anonymous reaction updates do not contain absolute totals, reactions
that existed before startup cannot be reconstructed reliably. Test with a new
message and add reactions while the bot is running.

Duplicate updates must not apply the same delta twice. Keep tracking
concurrency-safe, keep Telegram DTOs outside the moderation package, and never
log reacting users or other unnecessary personal information.

## Language and tooling

- Language: Go.
- Use Go modules.
- Format all Go code with `gofmt`.
- Prefer the Go standard library where practical.
- Avoid unnecessary dependencies.
- Before adding a new production dependency, explain why it is needed.
- Keep packages small and responsibilities explicit.

## Project structure

Prefer this structure:

cmd/bot/
main.go

internal/app/
internal/config/
internal/telegram/
internal/moderation/
internal/storage/

The Telegram API transport must be separated from moderation logic.

Moderation logic should be testable without making Telegram API requests.

## Testing

After modifying Go code, run:

    gofmt -w .
    go test ./...
    go vet ./...

Add unit tests for moderation rules.

Important cases include:

- positive reaction added and removed;
- negative reaction added and removed;
- positive reaction replaced with negative and vice versa;
- unknown reactions are ignored;
- threshold reached;
- threshold not reached;
- duplicate update IDs do not apply vote changes twice;
- different messages maintain independent totals;
- message deletion receives the correct chat and message identifiers.

Run tests before considering a task complete.

## Telegram integration

Do not assume Telegram Bot API behavior from memory when behavior may have
changed. Check the current Telegram Bot API documentation when necessary.

Keep Telegram-specific DTOs and API calls inside the Telegram integration
package.

The moderation package must not depend directly on Telegram API types.

## Configuration

Configuration must come from environment variables or configuration files.

Expected environment variables may include:

    TELEGRAM_BOT_TOKEN
    MODERATION_DISLIKE_THRESHOLD
    MODERATION_LIKE_THRESHOLD
    TELEGRAM_POLL_TIMEOUT
    DRY_RUN

Runtime defaults:

    MODERATION_DISLIKE_THRESHOLD=3
    MODERATION_LIKE_THRESHOLD=3
    TELEGRAM_POLL_TIMEOUT=30
    DRY_RUN=true

Validate configuration before starting long polling. Dry-run mode must execute
the full moderation decision flow without calling Telegram `deleteMessage`.

Do not put real tokens, passwords, API keys, chat IDs, or other secrets
into source code.

Never commit `.env`.

Provide `.env.example` containing names only, without real secrets.

## Security

Never print TELEGRAM_BOT_TOKEN in logs.

Never commit credentials.

Do not modify or remove security-related configuration without explaining
the change first.

## Codex workflow

Before making substantial changes:

1. Inspect the existing project structure.
2. Explain the intended change briefly.
3. Prefer the smallest coherent implementation.
4. Add or update tests.
5. Run formatting and tests.
6. Report what changed and what remains.

Do not implement unrelated features while working on a task.
