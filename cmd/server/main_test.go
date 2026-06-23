package main

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestShouldRunSetupWizardSkipsGatewayLiteMode(t *testing.T) {
	if shouldRunSetupWizard(config.RunModeGatewayLite, true) {
		t.Fatal("gateway-lite mode must not enter setup wizard")
	}
	if shouldRunSetupWizard("GATEWAY-LITE", true) {
		t.Fatal("gateway-lite mode normalization must not enter setup wizard")
	}
}

func TestShouldRunSetupWizardKeepsStandardSetup(t *testing.T) {
	if !shouldRunSetupWizard(config.RunModeStandard, true) {
		t.Fatal("standard mode should still enter setup wizard when setup is needed")
	}
	if shouldRunSetupWizard(config.RunModeStandard, false) {
		t.Fatal("standard mode should skip setup wizard when setup is not needed")
	}
}

func TestResolveRunModeForSetupDefaultsToGatewayLite(t *testing.T) {
	t.Setenv("RUN_MODE", "")
	if got := resolveRunModeForSetup(); got != config.RunModeGatewayLite {
		t.Fatalf("default run mode = %q, want gateway-lite", got)
	}
}
