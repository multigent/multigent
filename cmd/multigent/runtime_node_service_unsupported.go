//go:build !linux && !darwin

package main

import (
	"fmt"
	"runtime"
)

func servicePlatformName() string { return runtime.GOOS }

func runtimeNodeServiceInstallPlatform(runtimeNodeServiceConfig) error {
	return fmt.Errorf("runtime node service management is not supported on %s yet; use `multigent runtime start --daemon` or a native process manager", runtime.GOOS)
}

func runtimeNodeServiceUninstallPlatform() error {
	return fmt.Errorf("runtime node service management is not supported on %s yet", runtime.GOOS)
}

func runtimeNodeServiceStartPlatform() error {
	return fmt.Errorf("runtime node service management is not supported on %s yet", runtime.GOOS)
}

func runtimeNodeServiceStopPlatform() error {
	return fmt.Errorf("runtime node service management is not supported on %s yet", runtime.GOOS)
}

func runtimeNodeServiceRestartPlatform() error {
	return fmt.Errorf("runtime node service management is not supported on %s yet", runtime.GOOS)
}

func runtimeNodeServiceStatusPlatform() (*runtimeNodeServiceStatus, error) {
	return &runtimeNodeServiceStatus{Supported: false, Platform: runtime.GOOS, Error: "native service management is not supported yet; use `multigent runtime start --daemon`"}, nil
}
