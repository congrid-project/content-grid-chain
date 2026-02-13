package registry

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	bls "github.com/drand/kyber-bls12381"
	"github.com/drand/kyber/sign/tbls"
)

const (
	drandSchemePedersenUnchained = "pedersen-bls-unchained"
	drandSchemeShortSigG1        = "bls-unchained-on-g1"
	drandSchemeRFC9380G1         = "bls-unchained-g1-rfc9380"
)

func isSupportedDrandSchemeID(schemeID string) bool {
	switch strings.TrimSpace(schemeID) {
	case drandSchemePedersenUnchained, drandSchemeShortSigG1, drandSchemeRFC9380G1:
		return true
	default:
		return false
	}
}

func verifyDrandBeaconSignature(round uint64, signatureHex, randomnessHex, publicKeyHex, schemeID string) error {
	sig, err := hex.DecodeString(strings.TrimSpace(signatureHex))
	if err != nil {
		return fmt.Errorf("decode drand signature: %w", err)
	}
	randBytes, err := hex.DecodeString(strings.TrimSpace(randomnessHex))
	if err != nil {
		return fmt.Errorf("decode drand randomness: %w", err)
	}
	computedRand := sha256.Sum256(sig)
	if !equalBytes(computedRand[:], randBytes) {
		return fmt.Errorf("drand randomness mismatch")
	}

	pubBytes, err := hex.DecodeString(strings.TrimSpace(publicKeyHex))
	if err != nil {
		return fmt.Errorf("decode drand public key: %w", err)
	}

	schemeID = strings.TrimSpace(schemeID)
	if schemeID == "" {
		schemeID = DefaultDrandSchemeID
	}

	msg := digestUnchainedRound(round)
	switch schemeID {
	case drandSchemePedersenUnchained:
		pairing := bls.NewBLS12381SuiteWithDST(
			[]byte("BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_NUL_"),
			[]byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_"),
		)
		pub := pairing.G1().Point()
		if err := pub.UnmarshalBinary(pubBytes); err != nil {
			return fmt.Errorf("unmarshal drand public key (G1): %w", err)
		}
		if err := tbls.NewThresholdSchemeOnG2(pairing).VerifyRecovered(pub, msg, sig); err != nil {
			return fmt.Errorf("verify drand signature (%s): %w", schemeID, err)
		}
		return nil
	case drandSchemeShortSigG1:
		pairing := bls.NewBLS12381SuiteWithDST(
			[]byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_"),
			[]byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_"),
		)
		pub := pairing.G2().Point()
		if err := pub.UnmarshalBinary(pubBytes); err != nil {
			return fmt.Errorf("unmarshal drand public key (G2): %w", err)
		}
		if err := tbls.NewThresholdSchemeOnG1(pairing).VerifyRecovered(pub, msg, sig); err != nil {
			return fmt.Errorf("verify drand signature (%s): %w", schemeID, err)
		}
		return nil
	case drandSchemeRFC9380G1:
		pairing := bls.NewBLS12381SuiteWithDST(
			[]byte("BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_NUL_"),
			[]byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_"),
		)
		pub := pairing.G2().Point()
		if err := pub.UnmarshalBinary(pubBytes); err != nil {
			return fmt.Errorf("unmarshal drand public key (G2): %w", err)
		}
		if err := tbls.NewThresholdSchemeOnG1(pairing).VerifyRecovered(pub, msg, sig); err != nil {
			return fmt.Errorf("verify drand signature (%s): %w", schemeID, err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported drand scheme id: %s", schemeID)
	}
}

func digestUnchainedRound(round uint64) []byte {
	h := sha256.New()
	_ = binary.Write(h, binary.BigEndian, round)
	return h.Sum(nil)
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
