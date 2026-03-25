package checker

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
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
	cfg    *config.Config
	mu     sync.RWMutex
	status map[string]DeliveryStatus
}

func NewNotifier(cfg *config.Config) *Notifier {
	return &Notifier{
		cfg: cfg,
		status: map[string]DeliveryStatus{
			"webhook":  {Channel: "webhook"},
			"telegram": {Channel: "telegram"},
			"email":    {Channel: "email"},
		},
	}
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
	subject := buildSubject(cfg.Notifications.Email.SubjectPrefix, domain, check)
	timeout := notificationTimeout(cfg)

	if cfg.Notifications.Webhook.Enabled && strings.TrimSpace(cfg.Notifications.Webhook.URL) != "" && shouldNotify(check.OverallStatus, cfg.Notifications.Webhook.OnCritical, cfg.Notifications.Webhook.OnWarning) {
		go n.dispatch("webhook", true, func() error {
			return sendWebhook(cfg.Notifications.Webhook.URL, message, timeout)
		})
	}

	if cfg.Notifications.Telegram.Enabled && cfg.Notifications.Telegram.BotToken != "" && cfg.Notifications.Telegram.ChatID != "" && shouldNotify(check.OverallStatus, cfg.Notifications.Telegram.OnCritical, cfg.Notifications.Telegram.OnWarning) {
		go n.dispatch("telegram", true, func() error {
			return sendTelegram(cfg.Notifications.Telegram.BotToken, cfg.Notifications.Telegram.ChatID, message, timeout)
		})
	}

	if cfg.Notifications.Email.Enabled && len(cfg.Notifications.Email.To) > 0 && shouldNotify(check.OverallStatus, cfg.Notifications.Email.OnCritical, cfg.Notifications.Email.OnWarning) {
		go n.dispatch("email", true, func() error {
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

func (n *Notifier) SendTest() []TestResult {
	cfg := n.cfg.Snapshot()
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

	results := make([]TestResult, 0, 3)
	for _, channel := range []string{"webhook", "telegram", "email"} {
		result := TestResult{Channel: channel, Enabled: channelEnabled(cfg, channel)}
		if !result.Enabled {
			result.Error = "channel is disabled or not configured"
			results = append(results, result)
			continue
		}

		var err error
		switch channel {
		case "webhook":
			err = sendWebhook(cfg.Notifications.Webhook.URL, message, timeout)
		case "telegram":
			err = sendTelegram(cfg.Notifications.Telegram.BotToken, cfg.Notifications.Telegram.ChatID, message, timeout)
		case "email":
			err = sendEmail(cfg.Notifications.Email, subject, message, timeout)
		}
		n.recordDelivery(channel, result.Enabled, err)
		result.Success = err == nil
		if err != nil {
			result.Error = err.Error()
		}
		results = append(results, result)
	}
	return results
}

func (n *Notifier) dispatch(channel string, enabled bool, fn func() error) {
	err := fn()
	n.recordDelivery(channel, enabled, err)
	if err != nil {
		log.Printf("[notify] %s error: %v", channel, err)
		return
	}
	log.Printf("[notify] %s sent", channel)
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

func shouldNotify(status string, onCritical, onWarning bool) bool {
	switch status {
	case "critical":
		return onCritical
	case "warning":
		return onWarning
	default:
		return false
	}
}

func buildMessage(domain string, check *db.Check, prevStatus string) string {
	lines := []string{
		"SSL Domain Exporter Alert",
		"",
		fmt.Sprintf("Domain: %s", domain),
		fmt.Sprintf("Status: %s -> %s", prevStatus, check.OverallStatus),
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

func buildSubject(prefix, domain string, check *db.Check) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "[SSL Domain Exporter]"
	}
	return fmt.Sprintf("%s %s status=%s", prefix, domain, strings.ToUpper(check.OverallStatus))
}

func sendWebhook(url, message string, timeout time.Duration) error {
	payload, _ := json.Marshal(ginH{"text": message})
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("webhook failed, status: %d", resp.StatusCode)
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
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("telegram failed, status: %d", resp.StatusCode)
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
	defer client.Quit()
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
	defer client.Quit()

	if startTLS {
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

type ginH map[string]any
