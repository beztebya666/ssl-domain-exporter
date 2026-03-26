package checker

import (
	"context"
	"net/http"
	"testing"
	"time"

	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
)

func TestNotifierDeliverRetriesTransientHTTPFailures(t *testing.T) {
	notifier := NewNotifier(config.Default())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = notifier.Stop(ctx)
	})
	notifier.sleep = func(time.Duration) {}

	attempts := 0
	err := notifier.deliver("webhook", true, func() error {
		attempts++
		if attempts < 3 {
			return &notificationHTTPError{channel: "webhook", statusCode: http.StatusBadGateway}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected delivery to succeed after retries, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
	status := notifier.Status()[0]
	if status.LastSuccessAt == nil {
		t.Fatal("expected successful delivery timestamp to be recorded")
	}
	if status.LastError != "" {
		t.Fatalf("expected last error to be cleared, got %q", status.LastError)
	}
}

func TestNotifierDeliverDoesNotRetryPermanentHTTPFailures(t *testing.T) {
	notifier := NewNotifier(config.Default())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = notifier.Stop(ctx)
	})
	notifier.sleep = func(time.Duration) {}

	attempts := 0
	err := notifier.deliver("webhook", true, func() error {
		attempts++
		return &notificationHTTPError{channel: "webhook", statusCode: http.StatusBadRequest}
	})
	if err == nil {
		t.Fatal("expected permanent failure to be returned")
	}
	if attempts != 1 {
		t.Fatalf("expected permanent failure to avoid retries, got %d attempts", attempts)
	}
	status := notifier.Status()[0]
	if status.LastAttemptAt == nil {
		t.Fatal("expected last attempt timestamp to be recorded")
	}
	if status.LastError == "" {
		t.Fatal("expected last error to be recorded")
	}
}

func TestNotifierEnqueueDropsWhenQueueIsFull(t *testing.T) {
	notifier := &Notifier{
		status:         map[string]DeliveryStatus{"webhook": {Channel: "webhook"}},
		jobs:           make(chan notificationJob, 1),
		stopCh:         make(chan struct{}),
		workerDone:     make(chan struct{}),
		enqueueTimeout: 5 * time.Millisecond,
	}

	if accepted := notifier.enqueue("webhook", true, func() error { return nil }); !accepted {
		t.Fatal("expected first enqueue to succeed")
	}
	if accepted := notifier.enqueue("webhook", true, func() error { return nil }); accepted {
		t.Fatal("expected second enqueue to be dropped when the queue is full")
	}
	if got := len(notifier.jobs); got != 1 {
		t.Fatalf("expected queue to remain bounded at one item, got %d", got)
	}
}

func TestShouldNotifyTransitionSendsRecoveryAlerts(t *testing.T) {
	if !shouldNotifyTransition("critical", "ok", true, false) {
		t.Fatal("expected critical recovery to honor the critical notification toggle")
	}
	if !shouldNotifyTransition("warning", "ok", false, true) {
		t.Fatal("expected warning recovery to honor the warning notification toggle")
	}
	if shouldNotifyTransition("critical", "ok", false, false) {
		t.Fatal("expected recovery to stay disabled when channel policy is disabled")
	}
	if shouldNotifyTransition("unknown", "ok", true, true) {
		t.Fatal("expected unknown -> ok to avoid sending a recovery notification")
	}
}

func TestBuildSubjectMarksRecovery(t *testing.T) {
	subject := buildSubject("[SSL Domain Exporter]", "example.internal", &db.Check{OverallStatus: "ok"}, "critical")
	if subject != "[SSL Domain Exporter] example.internal recovered" {
		t.Fatalf("unexpected recovery subject: %q", subject)
	}
}

func TestNotifierStopDrainsQueuedJobs(t *testing.T) {
	notifier := NewNotifier(config.Default())
	notifier.sleep = func(time.Duration) {}

	delivered := make(chan struct{}, 1)
	if accepted := notifier.enqueue("webhook", true, func() error {
		delivered <- struct{}{}
		return nil
	}); !accepted {
		t.Fatal("expected enqueue to succeed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := notifier.Stop(ctx); err != nil {
		t.Fatalf("expected notifier to drain before shutdown, got %v", err)
	}

	select {
	case <-delivered:
	default:
		t.Fatal("expected queued notification to be delivered before shutdown completed")
	}
}

func TestNotifierSendTestTargetsSingleChannel(t *testing.T) {
	notifier := NewNotifier(config.Default())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = notifier.Stop(ctx)
	})

	results, err := notifier.SendTest(config.Default(), "email")
	if err != nil {
		t.Fatalf("send test: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].Channel != "email" {
		t.Fatalf("expected email result, got %+v", results[0])
	}
}

func TestNotifierSendTestRejectsUnknownChannel(t *testing.T) {
	notifier := NewNotifier(config.Default())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = notifier.Stop(ctx)
	})

	if _, err := notifier.SendTest(config.Default(), "pagerduty"); err == nil {
		t.Fatal("expected unknown channel to be rejected")
	}
}
