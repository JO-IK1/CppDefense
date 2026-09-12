package audit

import (
	"context"
	"errors"
	"time"
)

type Event struct {
	OccurredAt time.Time
	ActorID    *string
	ActorKind  string
	Action     string
	TargetType string
	TargetID   *string
	RequestID  string
	Reason     *string
	Metadata   map[string]any
}

type Repository interface {
	Append(context.Context, Event) error
}

type Service struct{ repository Repository }

func New(repository Repository) *Service {
	if repository == nil {
		panic("audit repository is required")
	}
	return &Service{repository: repository}
}

func (s *Service) Record(ctx context.Context, event Event) error {
	if event.Action == "" || event.ActorKind == "" || event.TargetType == "" || event.RequestID == "" {
		return errors.New("incomplete audit event")
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	return s.repository.Append(ctx, event)
}
