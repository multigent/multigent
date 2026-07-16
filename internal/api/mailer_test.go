package api

import "testing"

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

func TestNormalizeUploadedSkillContentDropsFrontmatter(t *testing.T) {
	got := normalizeUploadedSkillContent("---\nname: demo\ndescription: old\n---\n# Skill: Demo\n")
	if got != "# Skill: Demo" {
		t.Fatalf("normalized content=%q", got)
	}
}
