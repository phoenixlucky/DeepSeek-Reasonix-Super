package boot

import (
	"errors"
	"strings"

	"reasonix/internal/contract/event"
	"reasonix/internal/state/memory"
)

// memoryMigrationWarning reports a migration that left work undone. Unreadable
// facts are skipped, not deleted, so each is named; directory and index
// failures keep their own error text.
func memoryMigrationWarning(migration memory.MigrationReport, err error) (event.Event, bool) {
	if err == nil && len(migration.Unreadable) == 0 {
		return event.Event{}, false
	}
	ev := event.Event{Level: event.LevelWarn, Text: "Memory metadata migration did not complete."}
	var detail []string
	if len(migration.Unreadable) > 0 {
		detail = append(detail, "skipped unreadable: "+strings.Join(migration.Unreadable, ", "))
	}
	if err != nil {
		detail = append(detail, err.Error())
		var backupErr *memory.MigrationBackupError
		if errors.As(err, &backupErr) {
			ev.Code = event.NoticeCodeMemoryMigrationBackup
		}
	}
	ev.Detail = strings.Join(detail, "; ")
	return ev, true
}
