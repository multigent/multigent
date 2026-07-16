package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type CapabilityStatus struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

type Capabilities struct {
	Docker CapabilityStatus `json:"docker"`
	KVM    CapabilityStatus `json:"kvm"`
	E2B    CapabilityStatus `json:"e2b"`
}

func DetectCapabilities() Capabilities {
	docker := CapabilityStatus{Available: true}
	if err := CheckDocker(); err != nil {
		docker = CapabilityStatus{Available: false, Reason: err.Error()}
	}
	kvm := DetectKVM()
	e2b := CapabilityStatus{Available: kvm.Available}
	if !kvm.Available {
		e2b.Reason = "E2B self-hosted requires KVM: " + kvm.Reason
	} else if !e2bConfigured() {
		e2b.Available = false
		e2b.Reason = "KVM is available, but MULTIGENT_E2B_API_URL or E2B_API_URL is not configured"
	}
	return Capabilities{Docker: docker, KVM: kvm, E2B: e2b}
}

func DetectKVM() CapabilityStatus {
	if _, err := os.Stat("/dev/kvm"); err != nil {
		return CapabilityStatus{Available: false, Reason: "/dev/kvm is not present"}
	}
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		return CapabilityStatus{Available: false, Reason: fmt.Sprintf("/dev/kvm is not accessible: %v", err)}
	}
	_ = f.Close()
	if out, err := exec.Command("sh", "-c", "egrep -q '(vmx|svm)' /proc/cpuinfo").CombinedOutput(); err != nil {
		reason := strings.TrimSpace(string(out))
		if reason == "" {
			reason = "CPU virtualization flags were not detected"
		}
		return CapabilityStatus{Available: false, Reason: reason}
	}
	return CapabilityStatus{Available: true}
}

func e2bConfigured() bool {
	return strings.TrimSpace(os.Getenv("MULTIGENT_E2B_API_URL")) != "" ||
		strings.TrimSpace(os.Getenv("E2B_API_URL")) != ""
}
