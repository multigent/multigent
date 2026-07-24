package api

import (
	"strings"
	"testing"
)

func TestLoadSMTPConfigDisabledWithoutRequiredFields(t *testing.T) {
	t.Setenv("MULTIGENT_SMTP_HOST", "smtp.example.com")
	t.Setenv("MULTIGENT_SMTP_FROM", "")
	if _, ok := loadSMTPConfig(); ok {
		t.Fatalf("smtp config should be disabled without from address")
	}
}

func TestLoadSMTPConfigDefaults(t *testing.T) {
	t.Setenv("MULTIGENT_SMTP_HOST", "smtp.example.com")
	t.Setenv("MULTIGENT_SMTP_FROM", "noreply@example.com")
	cfg, ok := loadSMTPConfig()
	if !ok {
		t.Fatalf("smtp config should be enabled")
	}
	if cfg.Port != 587 || cfg.TLSMode != "starttls" {
		t.Fatalf("defaults port=%d tls=%q", cfg.Port, cfg.TLSMode)
	}
}

func TestBuildInvitationMessageUsesHTMLAndLocalizedContent(t *testing.T) {
	cfg := smtpConfig{From: "noreply@example.com", FromName: "Multigent"}
	msg, err := cfg.buildInvitationMessage(invitationEmailData{
		To:            "invitee@example.com",
		DisplayName:   "Invitee",
		InviteURL:     "https://multigent.dev/invite/token",
		WorkspaceName: "Spaceship",
		InviterName:   "Owner",
		ExpiresAt:     "2026-07-31T12:00:00Z",
		Locale:        "zh-CN,zh;q=0.9",
	})
	if err != nil {
		t.Fatalf("build invitation message: %v", err)
	}
	for _, want := range []string{
		"Content-Type: multipart/alternative;",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Type: text/html; charset=UTF-8",
		"=?UTF-8?",
		"邀请你加入 Spaceship",
		"接受邀请",
		"https://multigent.dev/invite/token",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message missing %q:\n%s", want, msg)
		}
	}
}

func TestNormalizeUploadedSkillContentDropsFrontmatter(t *testing.T) {
	got := normalizeUploadedSkillContent("---\nname: demo\ndescription: old\n---\n# Skill: Demo\n")
	if got != "# Skill: Demo" {
		t.Fatalf("normalized content=%q", got)
	}
}
