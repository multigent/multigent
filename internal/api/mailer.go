package api

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

type smtpConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	FromName string
	TLSMode  string
}

func loadSMTPConfig() (smtpConfig, bool) {
	host := strings.TrimSpace(os.Getenv("MULTIGENT_SMTP_HOST"))
	from := strings.TrimSpace(os.Getenv("MULTIGENT_SMTP_FROM"))
	if host == "" || from == "" {
		return smtpConfig{}, false
	}
	port := 587
	if raw := strings.TrimSpace(os.Getenv("MULTIGENT_SMTP_PORT")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			port = n
		}
	}
	tlsMode := strings.ToLower(strings.TrimSpace(os.Getenv("MULTIGENT_SMTP_TLS")))
	if tlsMode == "" {
		if port == 465 {
			tlsMode = "implicit"
		} else {
			tlsMode = "starttls"
		}
	}
	return smtpConfig{
		Host:     host,
		Port:     port,
		Username: strings.TrimSpace(os.Getenv("MULTIGENT_SMTP_USERNAME")),
		Password: os.Getenv("MULTIGENT_SMTP_PASSWORD"),
		From:     from,
		FromName: strings.TrimSpace(os.Getenv("MULTIGENT_SMTP_FROM_NAME")),
		TLSMode:  tlsMode,
	}, true
}

func (cfg smtpConfig) sendInvitation(to, displayName, inviteURL string) error {
	if strings.TrimSpace(to) == "" || strings.TrimSpace(inviteURL) == "" {
		return fmt.Errorf("recipient and invite URL are required")
	}
	fromAddr := mail.Address{Name: cfg.FromName, Address: cfg.From}
	toAddr := mail.Address{Name: displayName, Address: to}
	subject := "You're invited to Multigent"
	body := fmt.Sprintf("You have been invited to join Multigent.\n\nOpen this link to accept the invitation:\n%s\n\nThis invitation link expires automatically.\n", inviteURL)
	msg := strings.Join([]string{
		"From: " + fromAddr.String(),
		"To: " + toAddr.String(),
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")
	return cfg.sendMail([]string{to}, []byte(msg))
}

func (cfg smtpConfig) sendMail(to []string, msg []byte) error {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
	if err != nil {
		return fmt.Errorf("connect smtp server: %w", err)
	}
	defer conn.Close()

	var client *smtp.Client
	if cfg.TLSMode == "implicit" || cfg.TLSMode == "tls" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12})
		if err := tlsConn.Handshake(); err != nil {
			return fmt.Errorf("smtp tls handshake: %w", err)
		}
		client, err = smtp.NewClient(tlsConn, cfg.Host)
	} else {
		client, err = smtp.NewClient(conn, cfg.Host)
	}
	if err != nil {
		return fmt.Errorf("create smtp client: %w", err)
	}
	defer client.Close()

	if cfg.TLSMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("smtp starttls: %w", err)
			}
		}
	}
	if cfg.Username != "" {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
				return fmt.Errorf("smtp auth: %w", err)
			}
		}
	}
	if err := client.Mail(cfg.From); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp rcpt %s: %w", rcpt, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close data: %w", err)
	}
	return client.Quit()
}
