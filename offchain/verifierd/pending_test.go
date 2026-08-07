package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	registrypb "content-grid-chain/x/registry/typespb"
)

func TestPendingRevealForAssignmentLoadsLegacyState(t *testing.T) {
	assignment := &registrypb.PublisherVerificationAssignment{
		RoundStartUnix: 3600,
		Domain:         "example.com",
	}
	verifier := "congrid1legacyverifier"
	evidenceHash := "legacy-observed-set-hash"
	nonce := "legacy-nonce"
	commitHash := registrypb.ComputeVerificationCommitHash(
		assignment.GetDomain(),
		assignment.GetRoundStartUnix(),
		verifier,
		true,
		evidenceHash,
		nonce,
	)

	agent := Agent{Cfg: Config{StateDir: t.TempDir(), VerifierAddress: verifier}}
	legacy := map[string]any{
		"key":              assignmentKey(assignment),
		"domain":           assignment.GetDomain(),
		"round_start_unix": assignment.GetRoundStartUnix(),
		"verifier":         verifier,
		"passed":           true,
		"evidence_hash":    evidenceHash,
		"nonce":            nonce,
		"commit_hash":      commitHash,
	}
	bz, err := json.Marshal(legacy)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(agent.pendingRevealPath(assignment), bz, 0o600))

	pending, found, err := agent.pendingRevealForAssignment(assignment)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, evidenceHash, pending.EvidenceHash)
	require.Zero(t, pending.MatchedSimilarDomains)
}

func TestPendingRevealForAssignmentBindsVerificationOwner(t *testing.T) {
	owner := "congrid1pendingowner"
	verifier := "congrid1pendingverifier"
	assignment := &registrypb.PublisherVerificationAssignment{
		RoundStartUnix:    7200,
		Domain:            "example.com",
		VerificationOwner: owner,
	}
	nonce := "owner-bound-nonce"
	commitHash := registrypb.ComputeVerificationCommitHashV2(
		assignment.GetDomain(),
		assignment.GetRoundStartUnix(),
		verifier,
		owner,
		true,
		"",
		nonce,
	)

	pending := pendingReveal{
		Key:               assignmentKey(assignment),
		Domain:            assignment.GetDomain(),
		RoundStartUnix:    assignment.GetRoundStartUnix(),
		Verifier:          verifier,
		VerificationOwner: owner,
		Passed:            true,
		Nonce:             nonce,
		CommitHash:        commitHash,
	}
	require.NoError(t, pending.validateForAssignment(assignment, verifier))

	pending.VerificationOwner = "congrid1otherowner"
	require.ErrorContains(t, pending.validateForAssignment(assignment, verifier), "verification owner mismatch")
}
