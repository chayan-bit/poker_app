package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/chayan-bit/poker_app/server/internal/fair"
)

func TestRunValidCommitmentHuman(t *testing.T) {
	seed, err := fair.NewSeed()
	if err != nil {
		t.Fatalf("NewSeed: %v", err)
	}
	commitment := seed.Commitment()

	var stdout, stderr bytes.Buffer
	code := run([]string{"--commitment", commitment, "--seed", seed.Hex()}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "VALID") {
		t.Errorf("expected VALID in output, got: %s", out)
	}
	if !strings.Contains(out, commitment) {
		t.Errorf("expected commitment echoed in output")
	}
	if !strings.Contains(out, "does NOT burn cards") {
		t.Errorf("expected deal-order explanation in output")
	}
}

func TestRunValidCommitmentJSON(t *testing.T) {
	seed, err := fair.NewSeed()
	if err != nil {
		t.Fatalf("NewSeed: %v", err)
	}
	commitment := seed.Commitment()

	var stdout, stderr bytes.Buffer
	code := run([]string{"--commitment", commitment, "--seed", seed.Hex(), "--json"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, `"valid":true`) {
		t.Errorf("expected valid:true in JSON output, got: %s", out)
	}
	if !strings.Contains(out, commitment) {
		t.Errorf("expected commitment in JSON output")
	}
	if !strings.Contains(out, `"deck":[`) {
		t.Errorf("expected deck array in JSON output")
	}
}

func TestRunMismatch(t *testing.T) {
	seed, err := fair.NewSeed()
	if err != nil {
		t.Fatalf("NewSeed: %v", err)
	}
	wrongCommitment := strings.Repeat("0", 64)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--commitment", wrongCommitment, "--seed", seed.Hex()}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "MISMATCH") {
		t.Errorf("expected MISMATCH in stderr, got: %s", stderr.String())
	}
	if stdout.String() != "" {
		t.Errorf("expected no stdout on mismatch (human mode), got: %s", stdout.String())
	}
}

func TestRunMismatchJSON(t *testing.T) {
	seed, err := fair.NewSeed()
	if err != nil {
		t.Fatalf("NewSeed: %v", err)
	}
	wrongCommitment := strings.Repeat("0", 64)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--commitment", wrongCommitment, "--seed", seed.Hex(), "--json"}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), `"valid":false`) {
		t.Errorf("expected valid:false in JSON output, got: %s", stdout.String())
	}
}

func TestRunBadFlags(t *testing.T) {
	cases := [][]string{
		{},
		{"--commitment", "abc"},
		{"--seed", "abc"},
		{"--commitment", "abc", "--seed", "not-hex-and-wrong-length"},
		{"--unknown-flag", "x"},
	}
	for _, args := range cases {
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr)
		if code == 0 {
			t.Errorf("run(%v): expected non-zero exit code", args)
		}
	}
}
