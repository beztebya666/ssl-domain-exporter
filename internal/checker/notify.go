package checker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"domain-ssl-checker/internal/config"
	"domain-ssl-checker/internal/db"
)

type Notifier struct {
	cfg *config.Config
}

func NewNotifier(cfg *config.Config) *Notifier {
	return &Notifier{cfg: cfg}
}

func (n *Notifier) Notify(domain string, check *db.Check, prevStatus string) {
	if !n.cfg.Features.Notifications {
		return
	}
	if check.OverallStatus == prevStatus {
		return
	}

	message := n.buildMessage(domain, check, prevStatus)

	if n.cfg.Notifications.Webhook.Enabled && n.cfg.Notifications.Webhook.URL != "" {
		if shouldNotifyStatus(check.OverallStatus, n.cfg.Notifications.Webhook.OnCritical, n.cfg.Notifications.Webhook.OnWarning) {
			go n.sendWebhook(message)
		}
	}

	if n.cfg.Notifications.Telegram.Enabled && n.cfg.Notifications.Telegram.BotToken != "" && n.cfg.Notifications.Telegram.ChatID != "" {
		if shouldNotifyStatus(check.OverallStatus, n.cfg.Notifications.Telegram.OnCritical, n.cfg.Notifications.Telegram.OnWarning) {
			go n.sendTelegram(message)
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

func (n *Notifier) buildMessage(domain string, check *db.Check, prevStatus string) string {
	lines := []string{
		"Domain Monitor Alert",
		"",
		fmt.Sprintf("Domain: %s", domain),
		fmt.Sprintf("Status: %s -> %s", prevStatus, check.OverallStatus),
	}

	if check.SSLExpiryDays != nil {
		lines = append(lines, fmt.Sprintf("SSL expires in: %d days", *check.SSLExpiryDays))
	}
	if check.DomainExpiryDays != nil {
		lines = append(lines, fmt.Sprintf("Domain expires in: %d days", *check.DomainExpiryDays))
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
	if check.DomainCheckError != "" {
		lines = append(lines, fmt.Sprintf("Domain error: %s", check.DomainCheckError))
	}
	lines = append(lines, fmt.Sprintf("Checked at: %s", check.CheckedAt.Format(time.RFC3339)))

	return strings.Join(lines, "\n")
}

func (n *Notifier) sendWebhook(message string) {
	payload, _ := json.Marshal(map[string]string{"text": message})
	resp, err := http.Post(n.cfg.Notifications.Webhook.URL, "application/json", bytes.NewReader(payload))
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

func (n *Notifier) sendTelegram(message string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.cfg.Notifications.Telegram.BotToken)
	payload, _ := json.Marshal(map[string]string{
		"chat_id": n.cfg.Notifications.Telegram.ChatID,
		"text":    message,
	})
	resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
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
