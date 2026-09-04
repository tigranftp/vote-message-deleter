package app

import "context"

// NewDeletionGate returns a message deleter that skips its delegate while
// dry-run is enabled.
func NewDeletionGate(deleter MessageDeleter, dryRun bool) MessageDeleter {
	return deletionGate{
		deleter: deleter,
		dryRun:  dryRun,
	}
}

type deletionGate struct {
	deleter MessageDeleter
	dryRun  bool
}

func (gate deletionGate) DeleteMessage(ctx context.Context, chatID int64, messageID int) error {
	if gate.dryRun {
		return nil
	}
	if gate.deleter == nil {
		return ErrMessageDeleterRequired
	}

	return gate.deleter.DeleteMessage(ctx, chatID, messageID)
}
