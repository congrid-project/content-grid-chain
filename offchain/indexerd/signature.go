package main

import (
	"encoding/hex"
)

const signatureAlgoFoldSignV1 = "fold-sign-v1"

// foldSignSignature computes a compact binary signature from an embedding vector.
//
// It "folds" the embedding dimensions into `bits` buckets (by i%bits) and sets each
// bit based on the sign of the accumulated bucket.
//
// This is a deterministic, inexpensive approximation useful for diversity/
// de-duplication heuristics. It is NOT a cryptographic commitment.
func foldSignSignature(vec []float64, bits int) []byte {
	if bits <= 0 {
		return nil
	}
	acc := make([]float64, bits)
	for i, v := range vec {
		acc[i%bits] += v
	}
	out := make([]byte, (bits+7)/8)
	for i := 0; i < bits; i++ {
		if acc[i] > 0 {
			// big-endian bit order within each byte.
			out[i/8] |= 1 << uint(7-(i%8))
		}
	}
	return out
}

func signatureHex(vec []float64, bits int) (hexStr string, algo string) {
	sig := foldSignSignature(vec, bits)
	if len(sig) == 0 {
		return "", signatureAlgoFoldSignV1
	}
	return hex.EncodeToString(sig), signatureAlgoFoldSignV1
}
