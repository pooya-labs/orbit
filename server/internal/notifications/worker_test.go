package notifications

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

type fakeWorkerObserver struct {
	worker string
	outcome string
	items int64
	err error
}
func (f *fakeWorkerObserver) StartWorker(ctx context.Context, worker string) (context.Context, func(string, int64, error)) {
	return ctx, func(outcome string, items int64, err error) {
		f.worker, f.outcome, f.items, f.err = worker, outcome, items, err
	}
}

type fakeDeliveryRepository struct {
	deliveries []store.AlertNotificationDelivery
	records []store.RecordAlertNotificationAttemptInput
	claimErr error
	recordErr error
}

func (r *fakeDeliveryRepository) ClaimAlertNotificationDeliveries(_ context.Context, limit, leaseSeconds int) ([]store.AlertNotificationDelivery, error) {
	if r.claimErr != nil {
		return nil, r.claimErr
	}
	if len(r.deliveries) > limit {
		return nil, nil
	}
	return append([]store.AlertNotificationDelivery(nil), r.deliveries...), nil
}

func (r *fakeDeliveryRepository) RecordAlertNotificationAttempt(_ context.Context, input store.RecordAlertNotificationAttemptInput) error {
	if r.recordErr != nil {
		return r.recordErr
	}
	r.records = append(r.records, input)
	return nil
}

type fakeSender struct {
	result SendResult
}

func (s fakeSender) Send(context.Context, store.AlertNotificationDelivery) SendResult {
	return s.result
}

func TestWorkerSchedulesRetryWithExponentialBackoff(t *testing.T) {
	repository := &fakeDeliveryRepository{deliveries: []store.AlertNotificationDelivery{{ID: "delivery-1", Attempts: 2}}}
	observer := &fakeWorkerObserver{}
	worker := NewWorker(repository, fakeSender{result: SendResult{ErrorCode: "network_error"}}, slog.Default(), time.Second, observer)
	now := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	worker.now = func() time.Time { return now }
	worker.dispatch(context.Background())
	if observer.worker != "notification_delivery" || observer.outcome != "ok" || observer.items != 1 || observer.err != nil {
		t.Fatalf("worker observation = %+v", observer)
	}

	if len(repository.records) != 1 {
		t.Fatalf("record count = %d, want 1", len(repository.records))
	}
	record := repository.records[0]
	if record.Outcome != "retry_scheduled" || record.AttemptNumber != 2 ||
		record.ErrorCode != "network_error" || !record.NextAttemptAt.Equal(now.Add(30*time.Second)) {
		t.Fatalf("retry record = %+v", record)
	}
}

func TestWorkerMarksPermanentFailuresDead(t *testing.T) {
	repository := &fakeDeliveryRepository{deliveries: []store.AlertNotificationDelivery{{ID: "delivery-1", Attempts: 1}}}
	worker := NewWorker(repository, fakeSender{result: SendResult{Permanent: true, ErrorCode: "upstream_rejected"}}, nil, 0)
	worker.dispatch(context.Background())
	if len(repository.records) != 1 || repository.records[0].Outcome != "dead" {
		t.Fatalf("permanent failure records = %+v", repository.records)
	}
}

func TestWorkerStopsAfterMaximumAttempts(t *testing.T) {
	repository := &fakeDeliveryRepository{deliveries: []store.AlertNotificationDelivery{{ID: "delivery-1", Attempts: maxDeliveryAttempts}}}
	worker := NewWorker(repository, fakeSender{result: SendResult{ErrorCode: "network_error"}}, nil, 0)
	worker.dispatch(context.Background())
	if len(repository.records) != 1 || repository.records[0].Outcome != "dead" ||
		repository.records[0].ErrorCode != "attempt_limit_reached" {
		t.Fatalf("exhausted delivery record = %+v", repository.records)
	}
}

func TestRetryDelayIsBounded(t *testing.T) {
	if retryDelay(1) != 15*time.Second || retryDelay(2) != 30*time.Second || retryDelay(3) != 60*time.Second {
		t.Fatal("retry delay does not use the expected exponential schedule")
	}
	if got := retryDelay(20); got > time.Hour {
		t.Fatalf("retry delay = %s; want at most one hour", got)
	}
}
