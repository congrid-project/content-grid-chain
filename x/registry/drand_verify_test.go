package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	bls12381 "github.com/drand/kyber-bls12381"
	//nolint:staticcheck // test uses compatible BLS signing helpers for recovered signature verification.
	signBls "github.com/drand/kyber/sign/bls"
	"github.com/drand/kyber/util/random"
)

func TestVerifyDrandBeaconSignature_PedersenUnchained(t *testing.T) {
	pairing := bls12381.NewBLS12381SuiteWithDST(
		[]byte("BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_NUL_"),
		[]byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_"),
	)
	priv := pairing.G1().Scalar().Pick(random.New())
	pub := pairing.G1().Point().Mul(priv, nil)
	pubBytes, err := pub.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal pub: %v", err)
	}

	round := uint64(12345)
	msg := digestUnchainedRound(round)
	sig, err := signBls.NewSchemeOnG2(pairing).Sign(priv, msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	rand := sha256.Sum256(sig)

	if err := verifyDrandBeaconSignature(
		round,
		hex.EncodeToString(sig),
		hex.EncodeToString(rand[:]),
		hex.EncodeToString(pubBytes),
		drandSchemePedersenUnchained,
	); err != nil {
		t.Fatalf("expected valid drand signature, got err: %v", err)
	}
}

func TestVerifyDrandBeaconSignature_RFC9380G1(t *testing.T) {
	pairing := bls12381.NewBLS12381SuiteWithDST(
		[]byte("BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_NUL_"),
		[]byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_"),
	)
	priv := pairing.G2().Scalar().Pick(random.New())
	pub := pairing.G2().Point().Mul(priv, nil)
	pubBytes, err := pub.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal pub: %v", err)
	}

	round := uint64(54321)
	msg := digestUnchainedRound(round)
	sig, err := signBls.NewSchemeOnG1(pairing).Sign(priv, msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	rand := sha256.Sum256(sig)

	if err := verifyDrandBeaconSignature(
		round,
		hex.EncodeToString(sig),
		hex.EncodeToString(rand[:]),
		hex.EncodeToString(pubBytes),
		drandSchemeRFC9380G1,
	); err != nil {
		t.Fatalf("expected valid drand signature, got err: %v", err)
	}
}

func TestVerifyDrandBeaconSignature_QuicknetRound42(t *testing.T) {
	const signatureHex = "95a9f9f5b231b7714de1553105d8ffdf3dcda24cfdb1e689319bccf79a9c8ce430a91b811fbfaf763900bc998b5d686a"
	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	randomness := sha256.Sum256(signature)
	if err := verifyDrandBeaconSignature(
		42,
		signatureHex,
		hex.EncodeToString(randomness[:]),
		DefaultDrandPublicKeyHex,
		DefaultDrandSchemeID,
	); err != nil {
		t.Fatalf("expected official quicknet round 42 to verify, got err: %v", err)
	}
}

func TestVerifyDrandBeaconSignature_InvalidRandomness(t *testing.T) {
	pairing := bls12381.NewBLS12381SuiteWithDST(
		[]byte("BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_NUL_"),
		[]byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_"),
	)
	priv := pairing.G1().Scalar().Pick(random.New())
	pub := pairing.G1().Point().Mul(priv, nil)
	pubBytes, err := pub.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal pub: %v", err)
	}

	round := uint64(100)
	msg := digestUnchainedRound(round)
	sig, err := signBls.NewSchemeOnG2(pairing).Sign(priv, msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	badRand := make([]byte, 32)
	copy(badRand, []byte("not_the_real_randomness_value_1234"))

	if err := verifyDrandBeaconSignature(
		round,
		hex.EncodeToString(sig),
		hex.EncodeToString(badRand),
		hex.EncodeToString(pubBytes),
		drandSchemePedersenUnchained,
	); err == nil {
		t.Fatalf("expected invalid drand randomness to fail")
	}
}
