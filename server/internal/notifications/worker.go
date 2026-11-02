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

type Worker struct {
	repository DeliveryRepository
	sender Sender
	logger *slog.Logger
	interval time.Duration
	now func() time.Time
}

func NewWorker(repository DeliveryRepository, sender Sender, logger *slog.Logger, interval time.Duration) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Worker{
		repository: repository,
		sender: sender,
		logger: logger,
		interval: interval,
		now: time.Now,
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

func (w *Worker) dispatch(ctx context.Context) {
	deliveries, err := w.repository.ClaimAlertNotificationDeliveries(
		ctx, maxDeliveriesPerTick, deliveryLeaseSeconds,
	)
	if err != nil {
		if ctx.Err() == nil {
			w.logger.Error("could not claim alert-notification deliveries", "error", err)
		}
		return
	}
	for _, delivery := range deliveries {
		if ctx.Err() != nil {
			return
		}
		result := w.sender.Send(ctx, delivery)
		if ctx.Err() != nil {
			// The lease will expire and another API replica can reclaim the item.
			return
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
		case result.Permanent || delivery.Attempts >= maxDeliveryAttempts:
			record.Outcome = "dead"
			if record.ErrorCode == "" {
				record.ErrorCode = "attempt_limit_reached"
			}
		default:
			record.Outcome = "retry_scheduled"
			record.NextAttemptAt = w.now().UTC().Add(retryDelay(delivery.Attempts))
			if record.ErrorCode == "" {
				record.ErrorCode = "delivery_failed"
			}
		}
		if err := w.repository.RecordAlertNotificationAttempt(ctx, record); err != nil {
			w.logger.Error("could not record alert-notification attempt",
				"delivery_id", delivery.ID, "attempt", delivery.Attempts, "error", err)
			continue
		}
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
