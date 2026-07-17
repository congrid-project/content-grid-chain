package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	bls12381 "github.com/drand/kyber-bls12381"
	//nolint:staticcheck // test uses compatible BLS signing helpers for recovered signature verification.
	signBls "github.com/drand/kyber/sign/bls"
	"github.com/drand/kyber/util/random"
	"github.com/stretchr/testify/require"

	typespb "content-grid-chain/x/registry/typespb"
)

func TestRequiredDrandRound(t *testing.T) {
	params := DefaultPublisherParams()
	params.DrandGenesisTimeUnix = 1_000
	params.DrandPeriodSeconds = 10
	params.DrandRoundOffsetSeconds = 20

	round, beaconUnix, err := requiredDrandRound(params, 1_100)
	require.NoError(t, err)
	require.Equal(t, uint64(9), round)
	require.Equal(t, int64(1_080), beaconUnix)
}

func TestSubmitDrandBeacon_ExactStrictAndSingle(t *testing.T) {
	keeper, ctx := setupKeeper(t)
	ctx = ctx.WithBlockTime(time.Unix(1_050, 0).UTC())

	requiredRound := uint64(9)
	publicKeyHex, randomnessHex, signatureHex := makeRFC9380Beacon(t, requiredRound)
	params := DefaultPublisherParams()
	params.RoundIntervalSeconds = 100
	params.AssignmentDelayMaxSeconds = 100
	params.DrandEnabled = true
	params.DrandSchemeID = drandSchemeRFC9380G1
	params.DrandPublicKeyHex = publicKeyHex
	params.DrandChainHash = hex.EncodeToString(make([]byte, sha256.Size))
	params.DrandGenesisTimeUnix = 1_000
	params.DrandPeriodSeconds = 10
	params.DrandRoundOffsetSeconds = 20
	require.NoError(t, keeper.SetParams(ctx, params))

	requirement, err := keeper.PendingDrandRequirement(ctx)
	require.NoError(t, err)
	require.True(t, requirement.Pending)
	require.False(t, requirement.Submitted)
	require.Equal(t, int64(1_100), requirement.RoundStartUnix)
	require.Equal(t, requiredRound, requirement.RequiredDrandRound)

	submitter := sdk.AccAddress([]byte("drand-submitter-addr")).String()
	server := NewMsgServerImpl(keeper)
	wrongRoundMsg := &typespb.MsgSubmitDrandBeacon{
		Submitter:     submitter,
		Round:         requiredRound - 1,
		RandomnessHex: stringsOfZeroes(64),
		SignatureHex:  "00",
	}
	_, err = server.SubmitDrandBeacon(sdk.WrapSDKContext(ctx), wrongRoundMsg)
	require.ErrorIs(t, err, ErrUnexpectedDrandRound)

	missingSignatureMsg := &typespb.MsgSubmitDrandBeacon{
		Submitter:     submitter,
		Round:         requiredRound,
		RandomnessHex: randomnessHex,
	}
	_, err = server.SubmitDrandBeacon(sdk.WrapSDKContext(ctx), missingSignatureMsg)
	require.ErrorIs(t, err, ErrInvalidDrandBeacon)

	validMsg := &typespb.MsgSubmitDrandBeacon{
		Submitter:     submitter,
		Round:         requiredRound,
		RandomnessHex: randomnessHex,
		SignatureHex:  signatureHex,
	}
	_, err = server.SubmitDrandBeacon(sdk.WrapSDKContext(ctx), validMsg)
	require.NoError(t, err)

	requirement, err = keeper.PendingDrandRequirement(ctx)
	require.NoError(t, err)
	require.True(t, requirement.Submitted)

	_, err = server.SubmitDrandBeacon(sdk.WrapSDKContext(ctx), validMsg)
	require.ErrorIs(t, err, ErrDrandBeaconAlreadySubmitted)

	require.NoError(t, keeper.assignNewRound(ctx))
	meta, found := keeper.GetRoundMeta(ctx, 1_100)
	require.True(t, found)
	require.Equal(t, requiredRound, meta.DrandRound)
	require.Equal(t, randomnessHex, meta.DrandRandomnessHex)
}

func makeRFC9380Beacon(t *testing.T, round uint64) (string, string, string) {
	t.Helper()
	pairing := bls12381.NewBLS12381SuiteWithDST(
		[]byte("BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_NUL_"),
		[]byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_"),
	)
	privateKey := pairing.G2().Scalar().Pick(random.New())
	publicKey := pairing.G2().Point().Mul(privateKey, nil)
	publicKeyBytes, err := publicKey.MarshalBinary()
	require.NoError(t, err)
	signature, err := signBls.NewSchemeOnG1(pairing).Sign(privateKey, digestUnchainedRound(round))
	require.NoError(t, err)
	randomness := sha256.Sum256(signature)
	return hex.EncodeToString(publicKeyBytes), hex.EncodeToString(randomness[:]), hex.EncodeToString(signature)
}

func stringsOfZeroes(count int) string {
	out := make([]byte, count)
	for i := range out {
		out[i] = '0'
	}
	return string(out)
}
