package config

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
)

// SyslogWriter sends log lines to a remote syslog server over TCP or UDP.
// It implements io.Writer so it can be used as an slog handler destination.
type SyslogWriter struct {
	network  string
	address  string
	tag      string
	facility int
	mu       sync.Mutex
	conn     net.Conn
}

// NewSyslogWriter creates a new syslog writer and opens a connection.
func NewSyslogWriter(cfg SyslogConfig) (*SyslogWriter, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("syslog is not enabled")
	}
	address := strings.TrimSpace(cfg.Address)
	if address == "" {
		return nil, fmt.Errorf("syslog address is required")
	}
	network := strings.ToLower(strings.TrimSpace(cfg.Network))
	if network != "tcp" && network != "udp" {
		network = "udp"
	}
	tag := strings.TrimSpace(cfg.Tag)
	if tag == "" {
		tag = "ssl-domain-exporter"
	}
	facility := parseSyslogFacility(cfg.Facility)

	w := &SyslogWriter{
		network:  network,
		address:  address,
		tag:      tag,
		facility: facility,
	}
	if err := w.connect(); err != nil {
		return nil, fmt.Errorf("syslog connect: %w", err)
	}
	return w, nil
}

func (w *SyslogWriter) connect() error {
	conn, err := net.DialTimeout(w.network, w.address, 5*time.Second)
	if err != nil {
		return err
	}
	w.conn = conn
	return nil
}

// Write implements io.Writer for syslog. Each call sends one syslog message.
func (w *SyslogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	msg := strings.TrimRight(string(p), "\n\r")
	// RFC 3164 format: <priority>tag: message
	priority := w.facility*8 + 6 // facility * 8 + severity INFO
	syslogMsg := fmt.Sprintf("<%d>%s: %s\n", priority, w.tag, msg)

	if w.conn == nil {
		if err := w.connect(); err != nil {
			return 0, fmt.Errorf("syslog reconnect: %w", err)
		}
	}

	_ = w.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, err := w.conn.Write([]byte(syslogMsg))
	if err != nil {
		// Try reconnect once
		_ = w.conn.Close()
		w.conn = nil
		if reconnErr := w.connect(); reconnErr != nil {
			return 0, fmt.Errorf("syslog write failed and reconnect failed: %w", err)
		}
		_ = w.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
		_, err = w.conn.Write([]byte(syslogMsg))
		if err != nil {
			return 0, fmt.Errorf("syslog write after reconnect: %w", err)
		}
	}

	return len(p), nil
}

// Close closes the syslog connection.
func (w *SyslogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.conn != nil {
		return w.conn.Close()
	}
	return nil
}

// TestSyslog validates that a syslog connection can be established and a test message sent.
func TestSyslog(cfg SyslogConfig) error {
	w, err := NewSyslogWriter(cfg)
	if err != nil {
		return err
	}
	defer w.Close()
	_, err = w.Write([]byte("SSL Domain Exporter syslog test message"))
	return err
}

// ConfigureSyslogHandler returns an slog.Handler that writes to the given syslog writer.
// If json is true, the handler outputs structured JSON; otherwise plain text.
func ConfigureSyslogHandler(w io.Writer, json bool) slog.Handler {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if json {
		return slog.NewJSONHandler(w, opts)
	}
	return slog.NewTextHandler(w, opts)
}

func parseSyslogFacility(facility string) int {
	switch strings.ToLower(strings.TrimSpace(facility)) {
	case "kern":
		return 0
	case "user":
		return 1
	case "mail":
		return 2
	case "daemon":
		return 3
	case "auth":
		return 4
	case "syslog":
		return 5
	case "lpr":
		return 6
	case "news":
		return 7
	case "local0":
		return 16
	case "local1":
		return 17
	case "local2":
		return 18
	case "local3":
		return 19
	case "local4":
		return 20
	case "local5":
		return 21
	case "local6":
		return 22
	case "local7":
		return 23
	default:
		return 16 // local0
	}
}
