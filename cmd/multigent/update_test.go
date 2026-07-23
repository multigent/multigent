package main

import "testing"

func TestResolveUpdateChannel(t *testing.T) {
	t.Setenv("MULTIGENT_UPDATE_CHANNEL", "")
	tests := []struct {
		name    string
		value   string
		pre     bool
		beta    bool
		want    updateChannel
		wantErr bool
	}{
		{name: "default", want: updateChannelRelease},
		{name: "stable alias", value: "stable", want: updateChannelRelease},
		{name: "pre alias", value: "pre", want: updateChannelPrerelease},
		{name: "pre release", value: "pre-release", want: updateChannelPrerelease},
		{name: "beta", value: "beta", want: updateChannelBeta},
		{name: "pre flag", pre: true, want: updateChannelPrerelease},
		{name: "beta flag wins", pre: true, beta: true, want: updateChannelBeta},
		{name: "invalid", value: "nightly", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveUpdateChannel(tt.value, tt.pre, tt.beta)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveUpdateChannel: %v", err)
			}
			if got != tt.want {
				t.Fatalf("channel=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveUpdateChannelFromEnv(t *testing.T) {
	t.Setenv("MULTIGENT_UPDATE_CHANNEL", "beta")
	got, err := resolveUpdateChannel("", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != updateChannelBeta {
		t.Fatalf("channel=%q, want %q", got, updateChannelBeta)
	}
}

func TestUpdateCheckDisabled(t *testing.T) {
	for _, value := range []string{"1", "true", "yes"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("MULTIGENT_NO_UPDATE_CHECK", value)
			if !updateCheckDisabled() {
				t.Fatalf("updateCheckDisabled() = false")
			}
		})
	}
}

func TestShouldPrintUpdateReminderForCommand(t *testing.T) {
	skipped := []string{"", "multigent update", "multigent check-update", "multigent version", "multigent schema"}
	for _, cmd := range skipped {
		if shouldPrintUpdateReminderForCommand(cmd) {
			t.Fatalf("should skip reminder for %q", cmd)
		}
	}
	if !shouldPrintUpdateReminderForCommand("multigent start") {
		t.Fatalf("expected reminder to be allowed for start")
	}
}
