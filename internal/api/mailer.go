package api

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html"
	"mime"
	"mime/multipart"
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

type invitationEmailData struct {
	To            string
	DisplayName   string
	InviteURL     string
	WorkspaceName string
	InviterName   string
	ExpiresAt     string
	Locale        string
}

func (cfg smtpConfig) sendInvitation(data invitationEmailData) error {
	to := strings.TrimSpace(data.To)
	inviteURL := strings.TrimSpace(data.InviteURL)
	if strings.TrimSpace(to) == "" || strings.TrimSpace(inviteURL) == "" {
		return fmt.Errorf("recipient and invite URL are required")
	}
	data.To = to
	data.InviteURL = inviteURL
	msg, err := cfg.buildInvitationMessage(data)
	if err != nil {
		return err
	}
	return cfg.sendMail([]string{to}, []byte(msg))
}

func (cfg smtpConfig) buildInvitationMessage(data invitationEmailData) (string, error) {
	fromAddr := mail.Address{Name: cfg.FromName, Address: cfg.From}
	toAddr := mail.Address{Name: strings.TrimSpace(data.DisplayName), Address: data.To}
	workspaceName := strings.TrimSpace(data.WorkspaceName)
	if workspaceName == "" {
		workspaceName = "Multigent workspace"
	}
	inviterName := strings.TrimSpace(data.InviterName)
	if inviterName == "" {
		inviterName = "A workspace administrator"
	}
	zh := strings.HasPrefix(strings.ToLower(strings.TrimSpace(data.Locale)), "zh")
	subject := fmt.Sprintf("You're invited to %s on Multigent", workspaceName)
	title := "Join your Multigent workspace"
	intro := fmt.Sprintf("%s invited you to join %s.", inviterName, workspaceName)
	action := "Accept invitation"
	expiry := "This invitation link expires automatically."
	plainIntro := intro
	if zh {
		subject = fmt.Sprintf("邀请你加入 Multigent 工作区：%s", workspaceName)
		title = "加入你的 Multigent 工作区"
		intro = fmt.Sprintf("%s 邀请你加入 %s。", inviterName, workspaceName)
		action = "接受邀请"
		expiry = "该邀请链接会自动过期。"
		plainIntro = intro
	}
	if exp := formatInvitationExpiry(data.ExpiresAt); exp != "" {
		if zh {
			expiry = "邀请有效期至 " + exp + "。"
		} else {
			expiry = "This invitation expires on " + exp + "."
		}
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	textPart, err := writer.CreatePart(map[string][]string{
		"Content-Type":              {"text/plain; charset=UTF-8"},
		"Content-Transfer-Encoding": {"8bit"},
	})
	if err != nil {
		return "", err
	}
	text := fmt.Sprintf("%s\n\n%s:\n%s\n\n%s\n", plainIntro, action, data.InviteURL, expiry)
	if _, err := textPart.Write([]byte(text)); err != nil {
		return "", err
	}
	htmlPart, err := writer.CreatePart(map[string][]string{
		"Content-Type":              {"text/html; charset=UTF-8"},
		"Content-Transfer-Encoding": {"8bit"},
	})
	if err != nil {
		return "", err
	}
	htmlBody := invitationHTML(title, intro, action, data.InviteURL, expiry)
	if _, err := htmlPart.Write([]byte(htmlBody)); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	headers := []string{
		"From: " + fromAddr.String(),
		"To: " + toAddr.String(),
		"Subject: " + mime.QEncoding.Encode("UTF-8", subject),
		"MIME-Version: 1.0",
		"Content-Type: multipart/alternative; boundary=" + writer.Boundary(),
	}
	return strings.Join(headers, "\r\n") + "\r\n\r\n" + body.String(), nil
}

func invitationHTML(title, intro, action, inviteURL, expiry string) string {
	escTitle := html.EscapeString(title)
	escIntro := html.EscapeString(intro)
	escAction := html.EscapeString(action)
	escURL := html.EscapeString(inviteURL)
	escExpiry := html.EscapeString(expiry)
	return fmt.Sprintf(`<!doctype html>
<html>
<body style="margin:0;background:#f4f7fb;padding:32px 16px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:#111827;">
  <div style="max-width:560px;margin:0 auto;background:#ffffff;border:1px solid #e5e7eb;border-radius:16px;overflow:hidden;">
    <div style="padding:28px 32px;border-bottom:1px solid #eef2f7;">
      <div style="display:inline-flex;align-items:center;justify-content:center;width:40px;height:40px;border-radius:12px;background:#0284c7;color:#ffffff;font-weight:700;font-size:20px;">M</div>
      <h1 style="margin:20px 0 0;font-size:24px;line-height:1.3;color:#0f172a;">%s</h1>
    </div>
    <div style="padding:28px 32px;">
      <p style="margin:0 0 24px;font-size:16px;line-height:1.7;color:#374151;">%s</p>
      <a href="%s" style="display:inline-block;border-radius:10px;background:#0284c7;color:#ffffff;text-decoration:none;padding:12px 18px;font-size:14px;font-weight:700;">%s</a>
      <p style="margin:24px 0 0;font-size:13px;line-height:1.6;color:#6b7280;">%s</p>
      <p style="margin:14px 0 0;font-size:12px;line-height:1.6;color:#9ca3af;word-break:break-all;">%s</p>
    </div>
  </div>
</body>
</html>`, escTitle, escIntro, escURL, escAction, escExpiry, escURL)
}

func formatInvitationExpiry(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.Local().Format("2006-01-02 15:04 MST")
	}
	return raw
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
