package typespb

import "testing"

func TestComputeRoundSeedDeterministic(t *testing.T) {
	s1 := ComputeRoundSeed("grid-1", 1700000000)
	s2 := ComputeRoundSeed("grid-1", 1700000000)
	if s1 != s2 {
		t.Fatalf("seed should be deterministic")
	}
	s3 := ComputeRoundSeed("grid-2", 1700000000)
	if s1 == s3 {
		t.Fatalf("different chain id should produce different seed")
	}
}

func TestComputeRoundSeedWithAnchorDeterministic(t *testing.T) {
	a1 := ComputeRoundSeedWithAnchor("grid-1", 1700000000, 100, []byte{0x01, 0x02, 0x03})
	a2 := ComputeRoundSeedWithAnchor("grid-1", 1700000000, 100, []byte{0x01, 0x02, 0x03})
	if a1 != a2 {
		t.Fatalf("anchored seed should be deterministic")
	}
	a3 := ComputeRoundSeedWithAnchor("grid-1", 1700000000, 101, []byte{0x01, 0x02, 0x03})
	if a1 == a3 {
		t.Fatalf("different anchor height should produce different seed")
	}
	a4 := ComputeRoundSeedWithAnchor("grid-1", 1700000000, 100, []byte{0x01, 0x02, 0x04})
	if a1 == a4 {
		t.Fatalf("different anchor hash should produce different seed")
	}
}

func TestComputeRoundSeedWithDrandDeterministic(t *testing.T) {
	a1 := ComputeRoundSeedWithDrand("grid-1", 1700000000, 100, []byte{0x01, 0x02, 0x03}, 12345, []byte{0xaa, 0xbb})
	a2 := ComputeRoundSeedWithDrand("grid-1", 1700000000, 100, []byte{0x01, 0x02, 0x03}, 12345, []byte{0xaa, 0xbb})
	if a1 != a2 {
		t.Fatalf("drand seed should be deterministic")
	}
	a3 := ComputeRoundSeedWithDrand("grid-1", 1700000000, 100, []byte{0x01, 0x02, 0x03}, 12346, []byte{0xaa, 0xbb})
	if a1 == a3 {
		t.Fatalf("different drand round should produce different seed")
	}
	a4 := ComputeRoundSeedWithDrand("grid-1", 1700000000, 100, []byte{0x01, 0x02, 0x03}, 12345, []byte{0xaa, 0xbc})
	if a1 == a4 {
		t.Fatalf("different drand randomness should produce different seed")
	}
}

func TestComputeAssignmentOffsetHourlyUsesMinuteSlots(t *testing.T) {
	seed := ComputeRoundSeed("grid-1", 1700000000)
	off := ComputeAssignmentOffsetSeconds(seed, "Example.com", 3600, 3600)
	if off < 0 || off >= 3600 {
		t.Fatalf("offset out of range: %d", off)
	}
	if off%60 != 0 {
		t.Fatalf("hourly offset must be minute-aligned, got %d", off)
	}
}

func TestComputeAssignmentOffsetFastRoundsUsesDelayMax(t *testing.T) {
	seed := ComputeRoundSeed("grid-1", 1700000000)
	off := ComputeAssignmentOffsetSeconds(seed, "example.com", 10, 2)
	if off < 0 || off >= 2 {
		t.Fatalf("offset should be constrained by assignment_delay_max_seconds, got %d", off)
	}

	off2 := ComputeAssignmentOffsetSeconds(seed, "example.com", 10, 0)
	if off2 < 0 || off2 >= 10 {
		t.Fatalf("offset should fall back to round interval when delay_max<=0, got %d", off2)
	}
}
