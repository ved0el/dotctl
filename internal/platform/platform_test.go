package platform

import "testing"

func TestOS(t *testing.T) {
	switch OS() {
	case "darwin", "linux", "windows":
	default:
		t.Errorf("unexpected OS %q", OS())
	}
}

func TestArch(t *testing.T) {
	if Arch() == "" {
		t.Error("Arch() returned empty string")
	}
}

func TestHomeDir(t *testing.T) {
	home, err := HomeDir()
	if err != nil {
		t.Fatalf("HomeDir: %v", err)
	}
	if home == "" {
		t.Error("HomeDir() returned empty string")
	}
}
