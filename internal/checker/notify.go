package checker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
)

type Notifier struct {
	cfg *config.Config
}

func NewNotifier(cfg *config.Config) *Notifier {
	return &Notifier{cfg: cfg}
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

	if cfg.Notifications.Webhook.Enabled && cfg.Notifications.Webhook.URL != "" {
		if shouldNotifyStatus(check.OverallStatus, cfg.Notifications.Webhook.OnCritical, cfg.Notifications.Webhook.OnWarning) {
			go sendWebhook(cfg.Notifications.Webhook.URL, message)
		}
	}

	if cfg.Notifications.Telegram.Enabled && cfg.Notifications.Telegram.BotToken != "" && cfg.Notifications.Telegram.ChatID != "" {
		if shouldNotifyStatus(check.OverallStatus, cfg.Notifications.Telegram.OnCritical, cfg.Notifications.Telegram.OnWarning) {
			go sendTelegram(cfg.Notifications.Telegram.BotToken, cfg.Notifications.Telegram.ChatID, message)
		}
	}
}

func shouldNotifyStatus(status string, onCritical bool, onWarning bool) bool {
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
		"Domain Monitor Alert",
		"",
		fmt.Sprintf("Domain: %s", domain),
		fmt.Sprintf("Status: %s -> %s", prevStatus, check.OverallStatus),
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

func sendWebhook(url string, message string) {
	payload, _ := json.Marshal(map[string]string{"text": message})
	resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("[notify] webhook error: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("[notify] webhook failed, status: %d", resp.StatusCode)
		return
	}
	log.Printf("[notify] webhook sent")
}

func sendTelegram(botToken, chatID, message string) {
	telegramURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	payload, _ := json.Marshal(map[string]string{
		"chat_id": chatID,
		"text":    message,
	})
	resp, err := http.Post(telegramURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("[notify] telegram error: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("[notify] telegram failed, status: %d", resp.StatusCode)
		return
	}
	log.Printf("[notify] telegram sent")
}
