package notifications

import (
	"context"
	"log/slog"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

const (
	maxDeliveriesPerTick = 5
	deliveryLeaseSeconds = 90
	maxDeliveryAttempts = 8
)

type DeliveryRepository interface {
	ClaimAlertNotificationDeliveries(context.Context, int, int) ([]store.AlertNotificationDelivery, error)
	RecordAlertNotificationAttempt(context.Context, store.RecordAlertNotificationAttemptInput) error
}

type WorkerObserver interface {
	StartWorker(context.Context, string) (context.Context, func(string, int64, error))
}

type Worker struct {
	repository DeliveryRepository
	sender Sender
	logger *slog.Logger
	interval time.Duration
	now func() time.Time
	observer WorkerObserver
}

func NewWorker(repository DeliveryRepository, sender Sender, logger *slog.Logger, interval time.Duration, observers ...WorkerObserver) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	var observer WorkerObserver
	if len(observers) > 0 {
		observer = observers[0]
	}
	return &Worker{
		repository: repository,
		sender: sender,
		logger: logger,
		interval: interval,
		now: time.Now,
		observer: observer,
	}
}

func (w *Worker) Run(ctx context.Context) {
	if w == nil || w.repository == nil || w.sender == nil {
		return
	}
	w.dispatch(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.dispatch(ctx)
		}
	}
}

func (w *Worker) dispatch(parent context.Context) {
	ctx := parent
	finish := func(string, int64, error) {}
	if w.observer != nil {
		ctx, finish = w.observer.StartWorker(parent, "notification_delivery")
	}
	outcome, items, err := w.dispatchOnce(ctx)
	finish(outcome, items, err)
}

func (w *Worker) dispatchOnce(ctx context.Context) (string, int64, error) {
	deliveries, err := w.repository.ClaimAlertNotificationDeliveries(
		ctx, maxDeliveriesPerTick, deliveryLeaseSeconds,
	)
	if err != nil {
		if ctx.Err() == nil {
			w.logger.Error("could not claim alert-notification deliveries", "error", err)
		}
		return "error", 0, err
	}
	var processed int64
	var firstErr error
	for _, delivery := range deliveries {
		if ctx.Err() != nil {
			return "cancelled", processed, ctx.Err()
		}
		result := w.sender.Send(ctx, delivery)
		if ctx.Err() != nil {
			// The lease will expire and another API replica can reclaim the item.
			return "cancelled", processed, ctx.Err()
		}
		record := store.RecordAlertNotificationAttemptInput{
			DeliveryID: delivery.ID,
			AttemptNumber: delivery.Attempts,
			HTTPStatus: result.HTTPStatus,
			ErrorCode: result.ErrorCode,
			NextAttemptAt: w.now().UTC(),
		}
		switch {
		case result.Delivered:
			record.Outcome = "delivered"
			record.ErrorCode = ""
		case result.Permanent:
			record.Outcome = "dead"
			if record.ErrorCode == "" {
				record.ErrorCode = "permanent_delivery_failure"
			}
		case delivery.Attempts >= maxDeliveryAttempts:
			record.Outcome = "dead"
			record.ErrorCode = "attempt_limit_reached"
		default:
			record.Outcome = "retry_scheduled"
			record.NextAttemptAt = w.now().UTC().Add(retryDelay(delivery.Attempts))
			if record.ErrorCode == "" {
				record.ErrorCode = "delivery_failed"
			}
		}
		if recordErr := w.repository.RecordAlertNotificationAttempt(ctx, record); recordErr != nil {
			w.logger.Error("could not record alert-notification attempt",
				"delivery_id", delivery.ID, "attempt", delivery.Attempts, "error", recordErr)
			if firstErr == nil {
				firstErr = recordErr
			}
			continue
		}
		processed++
		if record.Outcome == "delivered" {
			w.logger.Info("alert notification delivered",
				"workspace_id", delivery.WorkspaceID,
				"delivery_id", delivery.ID,
				"channel_id", delivery.ChannelID,
				"attempt", delivery.Attempts)
		} else if record.Outcome == "dead" {
			w.logger.Error("alert notification delivery stopped",
				"workspace_id", delivery.WorkspaceID,
				"delivery_id", delivery.ID,
				"channel_id", delivery.ChannelID,
				"attempt", delivery.Attempts,
				"error_code", record.ErrorCode)
		} else {
			w.logger.Warn("alert notification delivery scheduled for retry",
				"workspace_id", delivery.WorkspaceID,
				"delivery_id", delivery.ID,
				"channel_id", delivery.ChannelID,
				"attempt", delivery.Attempts,
				"error_code", record.ErrorCode,
				"next_attempt_at", record.NextAttemptAt)
		}
	}
	if firstErr != nil {
		return "error", processed, firstErr
	}
	return "ok", processed, nil
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := 15 * time.Second
	for index := 1; index < attempt; index++ {
		delay *= 2
		if delay >= time.Hour {
			return time.Hour
		}
	}
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}
