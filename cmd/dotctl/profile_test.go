package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ved0el/dotctl/internal/machine"
)

func loadMachine(t *testing.T, home string) machine.Config {
	t.Helper()
	cfg, err := machine.Load(filepath.Join(home, ".config", "dotctl"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func saveMachine(t *testing.T, home, repo string, profiles ...string) {
	t.Helper()
	if err := machine.Save(filepath.Join(home, ".config", "dotctl"), machine.Config{Repo: repo, Profiles: profiles}); err != nil {
		t.Fatal(err)
	}
}

func TestMutateProfilesAddPersists(t *testing.T) {
	home, repo := withSandbox(t)
	if err := os.MkdirAll(filepath.Join(repo, "profiles", "tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := mutateProfiles(&globals{}, []string{"tools"}, true); err != nil {
		t.Fatalf("add: %v", err)
	}
	if got := loadMachine(t, home).Profiles; len(got) != 1 || got[0] != "tools" {
		t.Errorf("got %v, want [tools]", got)
	}
}

func TestMutateProfilesAddRejectsUnknownProfile(t *testing.T) {
	withSandbox(t) // only profiles/base exists
	if err := mutateProfiles(&globals{}, []string{"does-not-exist"}, true); err == nil {
		t.Error("expected error adding a profile with no directory")
	}
}

func TestMutateProfilesPreservesOrderAndDedups(t *testing.T) {
	home, repo := withSandbox(t)
	for _, p := range []string{"tools", "develop"} {
		if err := os.MkdirAll(filepath.Join(repo, "profiles", p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	saveMachine(t, home, repo, "base")

	if err := mutateProfiles(&globals{}, []string{"tools", "base", "develop"}, true); err != nil {
		t.Fatalf("add: %v", err)
	}
	want := []string{"base", "tools", "develop"}
	if got := loadMachine(t, home).Profiles; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMutateProfilesRemove(t *testing.T) {
	home, repo := withSandbox(t)
	if err := os.MkdirAll(filepath.Join(repo, "profiles", "tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	saveMachine(t, home, repo, "base", "tools")

	if err := mutateProfiles(&globals{}, []string{"tools"}, false); err != nil {
		t.Fatalf("rm: %v", err)
	}
	if got := loadMachine(t, home).Profiles; len(got) != 1 || got[0] != "base" {
		t.Errorf("got %v, want [base]", got)
	}
}

// TestMutateProfilesRefusesRemovingLast covers the guard against persisting an
// empty profile set (the "not bootstrapped" sentinel apply/sync reject).
func TestMutateProfilesRefusesRemovingLast(t *testing.T) {
	home, repo := withSandbox(t)
	saveMachine(t, home, repo, "base")

	if err := mutateProfiles(&globals{}, []string{"base"}, false); err == nil {
		t.Error("expected error removing the last profile")
	}
	if got := loadMachine(t, home).Profiles; len(got) != 1 || got[0] != "base" {
		t.Errorf("config must be unchanged after a refused removal; got %v", got)
	}
}
