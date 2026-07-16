package sandbox

import "testing"

func TestDetectCapabilitiesReturnsStructuredStatuses(t *testing.T) {
	caps := DetectCapabilities()
	if caps.KVM.Available && caps.KVM.Reason != "" {
		t.Fatalf("available KVM should not include reason: %#v", caps.KVM)
	}
	if caps.E2B.Available && !caps.KVM.Available {
		t.Fatalf("E2B cannot be available when KVM is unavailable: %#v", caps)
	}
}
