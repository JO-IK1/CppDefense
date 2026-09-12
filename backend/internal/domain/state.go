package domain

import "fmt"

type UserStatus string

const (
	UserPending  UserStatus = "pending"
	UserActive   UserStatus = "active"
	UserRejected UserStatus = "rejected"
	UserBlocked  UserStatus = "blocked"
)

type ImportState string

const (
	ImportReceiving     ImportState = "receiving"
	ImportStored        ImportState = "stored"
	ImportValidating    ImportState = "validating"
	ImportReviewPending ImportState = "review_pending"
	ImportApproved      ImportState = "approved"
	ImportApplying      ImportState = "applying"
	ImportCompleted     ImportState = "completed"
	ImportRejected      ImportState = "rejected"
	ImportFailed        ImportState = "failed"
	ImportCancelled     ImportState = "cancelled"
)

func CanTransitionImport(from, to ImportState) bool {
	allowed := map[ImportState]map[ImportState]bool{
		ImportReceiving:     {ImportStored: true, ImportRejected: true, ImportCancelled: true},
		ImportStored:        {ImportValidating: true, ImportCancelled: true},
		ImportValidating:    {ImportReviewPending: true, ImportRejected: true, ImportFailed: true, ImportCancelled: true},
		ImportReviewPending: {ImportApproved: true, ImportRejected: true, ImportCancelled: true},
		ImportApproved:      {ImportApplying: true, ImportCancelled: true},
		ImportApplying:      {ImportCompleted: true, ImportFailed: true},
		ImportFailed:        {ImportValidating: true, ImportApplying: true},
	}
	return allowed[from][to]
}

func RequireImportTransition(from, to ImportState) error {
	if !CanTransitionImport(from, to) {
		return fmt.Errorf("INVALID_STATE_TRANSITION: import %s -> %s", from, to)
	}
	return nil
}
