package main

import "testing"

// TestRunDoctorReportsProblems: the sandbox repo has no .git checkout (and no
// profiles configured), so doctor must report at least one problem.
func TestRunDoctorReportsProblems(t *testing.T) {
	withSandbox(t)
	if err := runDoctor(&globals{}); err == nil {
		t.Error("expected doctor to report problems (repo is not a git checkout)")
	}
}
