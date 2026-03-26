package checker

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
)

type DeliveryStatus struct {
	Channel       string     `json:"channel"`
	Enabled       bool       `json:"enabled"`
	LastAttemptAt *time.Time `json:"last_attempt_at,omitempty"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}

type TestResult struct {
	Channel string `json:"channel"`
	Enabled bool   `json:"enabled"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type Notifier struct {
	cfg              *config.Config
	mu               sync.RWMutex
	stopOnce         sync.Once
	status           map[string]DeliveryStatus
	jobs             chan notificationJob
	stopCh           chan struct{}
	workerDone       chan struct{}
	enqueueTimeout   time.Duration
	retryBaseBackoff time.Duration
	retryMaxBackoff  time.Duration
	retryAttempts    int
	sleep            func(time.Duration)
}

type notificationJob struct {
	channel string
	enabled bool
	fn      func() error
}

type notificationHTTPError struct {
	channel    string
	statusCode int
}

func NewNotifier(cfg *config.Config) *Notifier {
	n := &Notifier{
		cfg: cfg,
		status: map[string]DeliveryStatus{
			"webhook":  {Channel: "webhook"},
			"telegram": {Channel: "telegram"},
			"email":    {Channel: "email"},
		},
		jobs:             make(chan notificationJob, 128),
		stopCh:           make(chan struct{}),
		workerDone:       make(chan struct{}),
		enqueueTimeout:   2 * time.Second,
		retryBaseBackoff: 500 * time.Millisecond,
		retryMaxBackoff:  5 * time.Second,
		retryAttempts:    3,
		sleep:            time.Sleep,
	}
	go n.runWorker()
	return n
}

func (n *Notifier) Notify(domain string, check *db.Check, prevStatus string) {
	cfg := n.cfg.Snapshot()

	if !cfg.Features.Notifications {
		return
	}
	if check.OverallStatus == prevStatus {
		return
	}

	message := buildMessage(domain, check, prevStatus)
	subject := buildSubject(cfg.Notifications.Email.SubjectPrefix, domain, check, prevStatus)
	timeout := notificationTimeout(cfg)

	if cfg.Notifications.Webhook.Enabled && strings.TrimSpace(cfg.Notifications.Webhook.URL) != "" && shouldNotifyTransition(prevStatus, check.OverallStatus, cfg.Notifications.Webhook.OnCritical, cfg.Notifications.Webhook.OnWarning) {
		n.enqueue("webhook", true, func() error {
			return sendWebhook(cfg.Notifications.Webhook.URL, message, timeout)
		})
	}

	if cfg.Notifications.Telegram.Enabled && cfg.Notifications.Telegram.BotToken != "" && cfg.Notifications.Telegram.ChatID != "" && shouldNotifyTransition(prevStatus, check.OverallStatus, cfg.Notifications.Telegram.OnCritical, cfg.Notifications.Telegram.OnWarning) {
		n.enqueue("telegram", true, func() error {
			return sendTelegram(cfg.Notifications.Telegram.BotToken, cfg.Notifications.Telegram.ChatID, message, timeout)
		})
	}

	if cfg.Notifications.Email.Enabled && len(cfg.Notifications.Email.To) > 0 && shouldNotifyTransition(prevStatus, check.OverallStatus, cfg.Notifications.Email.OnCritical, cfg.Notifications.Email.OnWarning) {
		n.enqueue("email", true, func() error {
			return sendEmail(cfg.Notifications.Email, subject, message, timeout)
		})
	}
}

func (n *Notifier) Status() []DeliveryStatus {
	cfg := n.cfg.Snapshot()
	n.mu.RLock()
	defer n.mu.RUnlock()

	out := make([]DeliveryStatus, 0, 3)
	for _, channel := range []string{"webhook", "telegram", "email"} {
		status := n.status[channel]
		status.Enabled = channelEnabled(cfg, channel)
		out = append(out, status)
	}
	return out
}

func (n *Notifier) SendTest(cfg *config.Config, channel string) ([]TestResult, error) {
	if cfg == nil {
		cfg = n.cfg.Snapshot()
	}
	channels, err := notificationChannels(channel)
	if err != nil {
		return nil, err
	}
	timeout := notificationTimeout(cfg)
	message := strings.Join([]string{
		"SSL Domain Exporter test notification",
		"",
		"This is a manually triggered delivery test from the administration UI.",
		fmt.Sprintf("Time: %s", time.Now().UTC().Format(time.RFC3339)),
	}, "\n")
	subject := strings.TrimSpace(cfg.Notifications.Email.SubjectPrefix)
	if subject == "" {
		subject = "[SSL Domain Exporter]"
	}
	subject += " Test notification"

	results := make([]TestResult, 0, len(channels))
	for _, channel := range channels {
		result := TestResult{Channel: channel, Enabled: channelTestEnabled(cfg, channel)}
		if !result.Enabled {
			result.Error = "channel is disabled or not configured"
			results = append(results, result)
			continue
		}

		var err error
		var fn func() error
		switch channel {
		case "webhook":
			fn = func() error {
				return sendWebhook(cfg.Notifications.Webhook.URL, message, timeout)
			}
		case "telegram":
			fn = func() error {
				return sendTelegram(cfg.Notifications.Telegram.BotToken, cfg.Notifications.Telegram.ChatID, message, timeout)
			}
		case "email":
			fn = func() error {
				return sendEmail(cfg.Notifications.Email, subject, message, timeout)
			}
		}
		err = n.deliver(channel, result.Enabled, fn)
		result.Success = err == nil
		if err != nil {
			result.Error = err.Error()
		}
		results = append(results, result)
	}
	return results, nil
}

func (e *notificationHTTPError) Error() string {
	if e == nil {
		return "notification request failed"
	}
	return fmt.Sprintf("%s failed, status: %d", e.channel, e.statusCode)
}

func (e *notificationHTTPError) Retryable() bool {
	if e == nil {
		return false
	}
	return e.statusCode == http.StatusRequestTimeout ||
		e.statusCode == http.StatusTooEarly ||
		e.statusCode == http.StatusTooManyRequests ||
		e.statusCode >= http.StatusInternalServerError
}

func (n *Notifier) runWorker() {
	defer close(n.workerDone)
	for {
		select {
		case job := <-n.jobs:
			_ = n.deliver(job.channel, job.enabled, job.fn)
		case <-n.stopSignal():
			for {
				select {
				case job := <-n.jobs:
					_ = n.deliver(job.channel, job.enabled, job.fn)
				default:
					return
				}
			}
		}
	}
}

func (n *Notifier) enqueue(channel string, enabled bool, fn func() error) bool {
	if n == nil {
		return false
	}
	job := notificationJob{channel: channel, enabled: enabled, fn: fn}
	timeout := n.enqueueTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	stopCh := n.stopSignal()
	select {
	case <-stopCh:
		slog.Warn("Notification dropped during shutdown", "channel", channel)
		return false
	default:
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case n.jobs <- job:
		return true
	case <-timer.C:
		slog.Warn("Notification queue full, dropping event", "channel", channel, "timeout", timeout.String())
		return false
	case <-stopCh:
		slog.Warn("Notification dropped during shutdown", "channel", channel)
		return false
	}
}

func (n *Notifier) deliver(channel string, enabled bool, fn func() error) error {
	err := n.retry(channel, fn)
	n.recordDelivery(channel, enabled, err)
	if err != nil {
		slog.Error("Notification delivery failed", "channel", channel, "error", err)
		return err
	}
	slog.Info("Notification sent", "channel", channel)
	return nil
}

func (n *Notifier) retry(channel string, fn func() error) error {
	if fn == nil {
		return nil
	}
	attempts := n.retryAttempts
	if attempts < 1 {
		attempts = 1
	}
	backoff := n.retryBaseBackoff
	if backoff <= 0 {
		backoff = 500 * time.Millisecond
	}
	maxBackoff := n.retryMaxBackoff
	if maxBackoff < backoff {
		maxBackoff = backoff
	}

	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		if attempt == attempts || !isRetryableNotificationError(err) {
			return err
		}
		slog.Warn("Notification delivery failed, retrying",
			"channel", channel,
			"attempt", attempt,
			"max_attempts", attempts,
			"backoff", backoff.String(),
			"error", err,
		)
		if !n.waitBackoff(backoff) {
			return err
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
	return err
}

func isRetryableNotificationError(err error) bool {
	if err == nil {
		return false
	}
	var statusErr *notificationHTTPError
	if errors.As(err, &statusErr) {
		return statusErr.Retryable()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func (n *Notifier) recordDelivery(channel string, enabled bool, err error) {
	now := time.Now().UTC()
	n.mu.Lock()
	defer n.mu.Unlock()

	status := n.status[channel]
	status.Channel = channel
	status.Enabled = enabled
	status.LastAttemptAt = &now
	if err != nil {
		status.LastError = err.Error()
		n.status[channel] = status
		return
	}

	status.LastSuccessAt = &now
	status.LastError = ""
	n.status[channel] = status
}

func (n *Notifier) Stop(ctx context.Context) error {
	if n == nil {
		return nil
	}
	n.stopOnce.Do(func() {
		close(n.stopCh)
	})
	if ctx == nil {
		<-n.workerDone
		return nil
	}
	select {
	case <-n.workerDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (n *Notifier) stopSignal() <-chan struct{} {
	if n == nil {
		return nil
	}
	return n.stopCh
}

func (n *Notifier) waitBackoff(delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	if n == nil {
		time.Sleep(delay)
		return true
	}
	stopCh := n.stopSignal()
	if n.sleep != nil {
		done := make(chan struct{})
		go func() {
			n.sleep(delay)
			close(done)
		}()
		select {
		case <-done:
			return true
		case <-stopCh:
			return false
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-stopCh:
		return false
	}
}

func shouldNotifyTransition(prevStatus, status string, onCritical, onWarning bool) bool {
	if status == prevStatus {
		return false
	}
	switch status {
	case "critical", "error":
		return onCritical
	case "warning":
		return onWarning
	case "ok":
		switch prevStatus {
		case "critical", "error":
			return onCritical
		case "warning":
			return onWarning
		}
	default:
		return false
	}
	return false
}

func buildMessage(domain string, check *db.Check, prevStatus string) string {
	title := "SSL Domain Exporter Alert"
	if isRecoveryTransition(prevStatus, check.OverallStatus) {
		title = "SSL Domain Exporter Recovery"
	}

	lines := []string{
		title,
		"",
		fmt.Sprintf("Domain: %s", domain),
		fmt.Sprintf("Status: %s -> %s", prevStatus, check.OverallStatus),
	}
	if isRecoveryTransition(prevStatus, check.OverallStatus) {
		lines = append(lines, fmt.Sprintf("Recovery: domain returned to OK from %s", prevStatus))
	}

	if check.PrimaryReasonText != "" {
		lines = append(lines, fmt.Sprintf("Primary reason: %s", check.PrimaryReasonText))
	}
	if len(check.StatusReasons) > 0 {
		lines = append(lines, "Reasons:")
		for _, reason := range check.StatusReasons {
			lines = append(lines, fmt.Sprintf("- [%s] %s", strings.ToUpper(reason.Severity), firstNonEmpty(reason.Detail, reason.Summary)))
		}
	}
	if check.SSLExpiryDays != nil {
		lines = append(lines, fmt.Sprintf("SSL expires in: %d days", *check.SSLExpiryDays))
	}
	if !check.RegistrationCheckSkipped {
		if check.DomainExpiryDays != nil {
			lines = append(lines, fmt.Sprintf("Domain expires in: %d days", *check.DomainExpiryDays))
		}
		if check.DomainCheckError != "" {
			lines = append(lines, fmt.Sprintf("Domain error: %s", check.DomainCheckError))
		}
	} else {
		lines = append(lines, "Domain registration: not checked (ssl_only)")
	}
	if check.HTTPStatusCode > 0 {
		lines = append(lines, fmt.Sprintf("HTTP status: %d", check.HTTPStatusCode))
	}
	if check.CipherGrade != "" {
		lines = append(lines, fmt.Sprintf("Cipher grade: %s", check.CipherGrade))
	}
	if check.OCSPStatus != "" {
		lines = append(lines, fmt.Sprintf("OCSP: %s", check.OCSPStatus))
	}
	if check.CRLStatus != "" {
		lines = append(lines, fmt.Sprintf("CRL: %s", check.CRLStatus))
	}
	if check.SSLCheckError != "" {
		lines = append(lines, fmt.Sprintf("SSL error: %s", check.SSLCheckError))
	}
	if check.DNSServerUsed != "" {
		lines = append(lines, fmt.Sprintf("DNS: %s", check.DNSServerUsed))
	}
	lines = append(lines, fmt.Sprintf("Checked at: %s", check.CheckedAt.Format(time.RFC3339)))

	return strings.Join(lines, "\n")
}

func buildSubject(prefix, domain string, check *db.Check, prevStatus string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "[SSL Domain Exporter]"
	}
	if isRecoveryTransition(prevStatus, check.OverallStatus) {
		return fmt.Sprintf("%s %s recovered", prefix, domain)
	}
	return fmt.Sprintf("%s %s status=%s", prefix, domain, strings.ToUpper(check.OverallStatus))
}

func isRecoveryTransition(prevStatus, status string) bool {
	return status == "ok" && prevStatus != "" && prevStatus != "ok" && prevStatus != "unknown"
}

func sendWebhook(url, message string, timeout time.Duration) error {
	payload, _ := json.Marshal(ginH{"text": message})
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("dispatch webhook request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return &notificationHTTPError{channel: "webhook", statusCode: resp.StatusCode}
	}
	return nil
}

func sendTelegram(botToken, chatID, message string, timeout time.Duration) error {
	telegramURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	payload, _ := json.Marshal(ginH{
		"chat_id": chatID,
		"text":    message,
	})
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, telegramURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("dispatch telegram request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return &notificationHTTPError{channel: "telegram", statusCode: resp.StatusCode}
	}
	return nil
}

func sendEmail(cfg config.EmailConfig, subject, message string, timeout time.Duration) error {
	recipients := make([]string, 0, len(cfg.To))
	for _, recipient := range cfg.To {
		recipient = strings.TrimSpace(recipient)
		if recipient != "" {
			recipients = append(recipients, recipient)
		}
	}
	if len(recipients) == 0 {
		return fmt.Errorf("email skipped: no recipients configured")
	}

	from := strings.TrimSpace(cfg.From)
	if from == "" {
		from = strings.TrimSpace(cfg.Username)
	}
	if from == "" {
		return fmt.Errorf("email skipped: from address is empty")
	}
	if _, err := mail.ParseAddress(from); err != nil {
		return fmt.Errorf("email skipped: invalid from address: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", strings.TrimSpace(cfg.Host), cfg.Port)
	msg := buildEmailMessage(from, recipients, subject, message)
	return dialAndSendSMTP(cfg, addr, from, recipients, msg, timeout)
}

func buildEmailMessage(from string, to []string, subject, body string) []byte {
	headers := []string{
		fmt.Sprintf("From: %s", from),
		fmt.Sprintf("To: %s", strings.Join(to, ", ")),
		fmt.Sprintf("Subject: %s", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}
	return []byte(strings.Join(headers, "\r\n"))
}

func dialAndSendSMTP(cfg config.EmailConfig, addr, from string, to []string, message []byte, timeout time.Duration) error {
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = "starttls"
	}

	switch mode {
	case "tls":
		return sendSMTPTLS(cfg, addr, from, to, message, timeout)
	case "none":
		return sendSMTPPlain(cfg, addr, from, to, message, false, timeout)
	default:
		return sendSMTPPlain(cfg, addr, from, to, message, true, timeout)
	}
}

func sendSMTPTLS(cfg config.EmailConfig, addr, from string, to []string, message []byte, timeout time.Duration) error {
	dialer := &net.Dialer{Timeout: timeout}
	//nolint:gosec // SMTP validation is admin-controlled; some environments require skipping PKI validation for internal mail relays.
	tlsConfig := &tls.Config{
		ServerName:         strings.TrimSpace(cfg.Host),
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	client, err := smtp.NewClient(conn, strings.TrimSpace(cfg.Host))
	if err != nil {
		return err
	}
	defer closeSMTPClient(client)
	return smtpSend(client, cfg, from, to, message)
}

func sendSMTPPlain(cfg config.EmailConfig, addr, from string, to []string, message []byte, startTLS bool, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	client, err := smtp.NewClient(conn, strings.TrimSpace(cfg.Host))
	if err != nil {
		return err
	}
	defer closeSMTPClient(client)

	if startTLS {
		//nolint:gosec // SMTP validation is admin-controlled; some environments require skipping PKI validation for internal mail relays.
		tlsConfig := &tls.Config{
			ServerName:         strings.TrimSpace(cfg.Host),
			InsecureSkipVerify: cfg.InsecureSkipVerify,
		}
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConfig); err != nil {
				return err
			}
		}
	}
	return smtpSend(client, cfg, from, to, message)
}

func smtpSend(client *smtp.Client, cfg config.EmailConfig, from string, to []string, message []byte) error {
	if strings.TrimSpace(cfg.Username) != "" {
		auth := smtp.PlainAuth("", strings.TrimSpace(cfg.Username), cfg.Password, strings.TrimSpace(cfg.Host))
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return err
			}
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}

func closeSMTPClient(client *smtp.Client) {
	if client == nil {
		return
	}
	if err := client.Quit(); err != nil {
		slog.Debug("SMTP client quit failed", "error", err)
	}
}

func notificationTimeout(cfg *config.Config) time.Duration {
	if cfg != nil {
		if timeout, err := time.ParseDuration(strings.TrimSpace(cfg.Notifications.Timeout)); err == nil && timeout > 0 {
			return timeout
		}
	}
	return 15 * time.Second
}

func channelEnabled(cfg *config.Config, channel string) bool {
	if cfg == nil || !cfg.Features.Notifications {
		return false
	}
	return channelTestEnabled(cfg, channel)
}

func channelTestEnabled(cfg *config.Config, channel string) bool {
	if cfg == nil {
		return false
	}
	switch channel {
	case "webhook":
		return cfg.Notifications.Webhook.Enabled && strings.TrimSpace(cfg.Notifications.Webhook.URL) != ""
	case "telegram":
		return cfg.Notifications.Telegram.Enabled && strings.TrimSpace(cfg.Notifications.Telegram.BotToken) != "" && strings.TrimSpace(cfg.Notifications.Telegram.ChatID) != ""
	case "email":
		return cfg.Notifications.Email.Enabled && strings.TrimSpace(cfg.Notifications.Email.Host) != "" && len(cfg.Notifications.Email.To) > 0
	default:
		return false
	}
}

func notificationChannels(channel string) ([]string, error) {
	normalized := strings.ToLower(strings.TrimSpace(channel))
	switch normalized {
	case "":
		return []string{"webhook", "telegram", "email"}, nil
	case "webhook", "telegram", "email":
		return []string{normalized}, nil
	default:
		return nil, fmt.Errorf("channel must be one of webhook, telegram, email")
	}
}

type ginH map[string]any
