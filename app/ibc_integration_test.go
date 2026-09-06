package app_test

import (
	"encoding/json"
	"testing"
	"time"

	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	abci "github.com/cometbft/cometbft/abci/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	transfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	clienttypes "github.com/cosmos/ibc-go/v10/modules/core/02-client/types"
	channeltypes "github.com/cosmos/ibc-go/v10/modules/core/04-channel/types"
	host "github.com/cosmos/ibc-go/v10/modules/core/24-host"
	ibckeeper "github.com/cosmos/ibc-go/v10/modules/core/keeper"
	ibctesting "github.com/cosmos/ibc-go/v10/testing"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"content-grid-chain/app"
	"content-grid-chain/x/registry"
	"content-grid-chain/x/tokenomics"
)

// This adapter runs the production application through ibc-go's signed ABCI
// transactions and Tendermint/Merkle proof verification. It only adapts genesis
// funding because ibc-go's fixture hardcodes the SDK's "stake" denomination.
type ibcTestingApp struct{ *app.App }

func (a *ibcTestingApp) GetBaseApp() *baseapp.BaseApp    { return a.BaseApp }
func (a *ibcTestingApp) GetIBCKeeper() *ibckeeper.Keeper { return a.IBCKeeper }
func (a *ibcTestingApp) GetTxConfig() client.TxConfig    { return a.TxConfig() }

const testUSDCDenom = "uusdc-test" // Test asset only; never a real USDC representation.

func (a *ibcTestingApp) InitChain(req *abci.RequestInitChain) (*abci.ResponseInitChain, error) {
	var genesis map[string]json.RawMessage
	if err := json.Unmarshal(req.AppStateBytes, &genesis); err != nil {
		return nil, err
	}
	var bank banktypes.GenesisState
	a.AppCodec().MustUnmarshalJSON(genesis[banktypes.ModuleName], &bank)
	for i := range bank.Balances {
		coins := sdk.NewCoins()
		for _, coin := range bank.Balances[i].Coins {
			if coin.Denom == sdk.DefaultBondDenom {
				coin.Denom = tokenomics.DefaultDenom
			} else if coin.Denom == ibctesting.SecondaryDenom {
				coin.Denom = testUSDCDenom
			}
			coins = coins.Add(coin)
		}
		bank.Balances[i].Coins = coins
	}
	genesis[banktypes.ModuleName] = a.AppCodec().MustMarshalJSON(&bank)
	var err error
	req.AppStateBytes, err = json.Marshal(genesis)
	if err != nil {
		return nil, err
	}
	return a.App.InitChain(req)
}

func newIBCNetwork(t *testing.T) (*ibctesting.Coordinator, *ibctesting.Path) {
	t.Helper()
	// ibc-go's default clock starts in 2020, before Congrid's drand network.
	coord := &ibctesting.Coordinator{T: t, CurrentTime: time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), Chains: make(map[string]*ibctesting.TestChain)}
	creator := func() (ibctesting.TestingApp, map[string]json.RawMessage) {
		opts := viper.New()
		opts.Set("home", t.TempDir())
		a := app.NewApp(log.NewNopLogger(), dbm.NewMemDB(), nil, true, opts)
		t.Cleanup(func() { require.NoError(t, a.Close()) })
		genesis := app.DefaultGenesis()
		// Isolate IBC supply conservation from unrelated SDK block inflation.
		var mint minttypes.GenesisState
		a.AppCodec().MustUnmarshalJSON(genesis[minttypes.ModuleName], &mint)
		mint.Minter.Inflation = sdkmath.LegacyZeroDec()
		mint.Params.InflationRateChange = sdkmath.LegacyZeroDec()
		mint.Params.InflationMin = sdkmath.LegacyZeroDec()
		mint.Params.InflationMax = sdkmath.LegacyZeroDec()
		genesis[minttypes.ModuleName] = a.AppCodec().MustMarshalJSON(&mint)
		return &ibcTestingApp{a}, genesis
	}
	for i := 1; i <= 2; i++ {
		id := ibctesting.GetChainID(i)
		coord.Chains[id] = ibctesting.NewCustomAppTestChain(t, coord, id, creator)
	}
	path := ibctesting.NewTransferPath(coord.GetChain(ibctesting.GetChainID(1)), coord.GetChain(ibctesting.GetChainID(2)))
	path.Setup()
	for _, endpoint := range []*ibctesting.Endpoint{path.EndpointA, path.EndpointB} {
		channel, found := endpoint.Chain.App.GetIBCKeeper().ChannelKeeper.GetChannel(endpoint.Chain.GetContext(), transfertypes.PortID, endpoint.ChannelID)
		require.True(t, found)
		require.Equal(t, channeltypes.OPEN, channel.State)
		require.Equal(t, channeltypes.UNORDERED, channel.Ordering)
		require.Equal(t, transfertypes.V1, channel.Version)
	}
	return coord, path
}

func gridApp(chain *ibctesting.TestChain) *app.App { return chain.App.(*ibcTestingApp).App }

func balance(chain *ibctesting.TestChain, address sdk.AccAddress, denom string) sdkmath.Int {
	return gridApp(chain).BankKeeper.GetBalance(chain.GetContext(), address, denom).Amount
}

func sendIBC(t *testing.T, from *ibctesting.Endpoint, coin sdk.Coin, receiver string, timeout uint64) channeltypes.Packet {
	t.Helper()
	msg := transfertypes.NewMsgTransfer(transfertypes.PortID, from.ChannelID, coin,
		from.Chain.SenderAccount.GetAddress().String(), receiver, clienttypes.ZeroHeight(), timeout, "")
	result, err := from.Chain.SendMsgs(msg)
	require.NoError(t, err)
	packet, err := ibctesting.ParseV1PacketFromEvents(result.Events)
	require.NoError(t, err)
	return packet
}

func assertNoCommitment(t *testing.T, endpoint *ibctesting.Endpoint, packet channeltypes.Packet) {
	t.Helper()
	require.Empty(t, endpoint.Chain.App.GetIBCKeeper().ChannelKeeper.GetPacketCommitment(
		endpoint.Chain.GetContext(), packet.SourcePort, packet.SourceChannel, packet.Sequence))
}

func TestIBCTransferRoundTrip(t *testing.T) {
	coord, path := newIBCNetwork(t)
	for _, tc := range []struct {
		name string
		path *ibctesting.Path
		coin sdk.Coin
	}{
		{"CONGRID outbound and return", path, sdk.NewInt64Coin(tokenomics.DefaultDenom, 10_000_000)},
		{"1000 test USDC inbound and return", path.Reversed(), sdk.NewInt64Coin(testUSDCDenom, 1_000_000_000)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			from, to := tc.path.EndpointA, tc.path.EndpointB
			sender, receiver := from.Chain.SenderAccount.GetAddress(), to.Chain.SenderAccount.GetAddress()
			initial := balance(from.Chain, sender, tc.coin.Denom)
			supply := gridApp(from.Chain).BankKeeper.GetSupply(from.Chain.GetContext(), tc.coin.Denom)
			escrow := transfertypes.GetEscrowAddress(transfertypes.PortID, from.ChannelID)
			packet := sendIBC(t, from, tc.coin, receiver.String(), uint64(coord.CurrentTime.Add(time.Hour).UnixNano()))
			require.NoError(t, to.UpdateClient())
			proof, proofHeight := from.QueryProof(host.PacketCommitmentKey(packet.SourcePort, packet.SourceChannel, packet.Sequence))
			require.Equal(t, initial.Sub(tc.coin.Amount), balance(from.Chain, sender, tc.coin.Denom))
			require.Equal(t, tc.coin.Amount, balance(from.Chain, escrow, tc.coin.Denom))
			require.NoError(t, tc.path.RelayPacket(packet))
			assertNoCommitment(t, from, packet)

			denom := transfertypes.NewDenom(tc.coin.Denom, transfertypes.NewHop(transfertypes.PortID, to.ChannelID))
			voucher := sdk.NewCoin(denom.IBCDenom(), tc.coin.Amount)
			require.Equal(t, voucher.Amount, balance(to.Chain, receiver, voucher.Denom))
			trace, err := gridApp(to.Chain).TransferKeeper.GetDenomFromIBCDenom(to.Chain.GetContext(), voucher.Denom)
			require.NoError(t, err)
			require.Equal(t, denom, trace)
			// Replaying a received packet must never mint a second voucher.
			_, err = to.Chain.SendMsgs(channeltypes.NewMsgRecvPacket(packet, proof, proofHeight, receiver.String()))
			require.NoError(t, err)
			require.Equal(t, voucher.Amount, balance(to.Chain, receiver, voucher.Denom))

			back := sendIBC(t, to, voucher, sender.String(), uint64(coord.CurrentTime.Add(time.Hour).UnixNano()))
			require.True(t, balance(to.Chain, receiver, voucher.Denom).IsZero())
			require.NoError(t, tc.path.RelayPacket(back))
			assertNoCommitment(t, to, back)
			require.Equal(t, initial, balance(from.Chain, sender, tc.coin.Denom))
			require.True(t, balance(from.Chain, escrow, tc.coin.Denom).IsZero())
			require.True(t, gridApp(from.Chain).TransferKeeper.GetTotalEscrowForDenom(from.Chain.GetContext(), tc.coin.Denom).IsZero())
			require.True(t, gridApp(to.Chain).BankKeeper.GetSupply(to.Chain.GetContext(), voucher.Denom).IsZero())
			require.Equal(t, supply, gridApp(from.Chain).BankKeeper.GetSupply(from.Chain.GetContext(), tc.coin.Denom))
			t.Logf("round trip verified: %s -> %s; escrow, voucher supply and packet commitments cleared", tc.coin, voucher.Denom)
		})
	}
}

func TestIBCTransferRefunds(t *testing.T) {
	coord, path := newIBCNetwork(t)
	from, to := path.EndpointA, path.EndpointB
	sender := from.Chain.SenderAccount.GetAddress()
	coin := sdk.NewInt64Coin(tokenomics.DefaultDenom, 3_000_000)
	initial := balance(from.Chain, sender, coin.Denom)
	escrow := transfertypes.GetEscrowAddress(transfertypes.PortID, from.ChannelID)

	t.Run("error acknowledgement refunds sender", func(t *testing.T) {
		packet := sendIBC(t, from, coin, "invalid-receiver", uint64(coord.CurrentTime.Add(time.Hour).UnixNano()))
		_, ackBytes, err := path.RelayPacketWithResults(packet)
		require.NoError(t, err)
		var ack channeltypes.Acknowledgement
		require.NoError(t, json.Unmarshal(ackBytes, &ack))
		require.False(t, ack.Success())
		require.Equal(t, initial, balance(from.Chain, sender, coin.Denom))
		require.True(t, balance(from.Chain, escrow, coin.Denom).IsZero())
		assertNoCommitment(t, from, packet)
	})
	t.Run("timeout refunds sender using nonreceipt proof", func(t *testing.T) {
		packet := sendIBC(t, from, coin, to.Chain.SenderAccount.GetAddress().String(), uint64(coord.CurrentTime.Add(time.Minute).UnixNano()))
		require.Equal(t, initial.Sub(coin.Amount), balance(from.Chain, sender, coin.Denom))
		coord.IncrementTimeBy(2 * time.Minute)
		coord.CommitBlock(to.Chain)
		require.NoError(t, from.UpdateClient())
		require.NoError(t, from.TimeoutPacket(packet))
		require.Equal(t, initial, balance(from.Chain, sender, coin.Denom))
		require.True(t, balance(from.Chain, escrow, coin.Denom).IsZero())
		assertNoCommitment(t, from, packet)
	})
	t.Run("timed out voucher return remints the burned voucher", func(t *testing.T) {
		receiver := to.Chain.SenderAccount.GetAddress()
		outbound := sendIBC(t, from, coin, receiver.String(), uint64(coord.CurrentTime.Add(time.Hour).UnixNano()))
		require.NoError(t, path.RelayPacket(outbound))
		denom := transfertypes.NewDenom(coin.Denom, transfertypes.NewHop(transfertypes.PortID, to.ChannelID)).IBCDenom()
		voucher := sdk.NewCoin(denom, coin.Amount)
		back := sendIBC(t, to, voucher, sender.String(), uint64(coord.CurrentTime.Add(time.Minute).UnixNano()))
		require.True(t, balance(to.Chain, receiver, denom).IsZero())
		coord.IncrementTimeBy(2 * time.Minute)
		coord.CommitBlock(from.Chain)
		require.NoError(t, to.UpdateClient())
		require.NoError(t, to.TimeoutPacket(back))
		assertNoCommitment(t, to, back)
		require.Equal(t, coin.Amount, balance(to.Chain, receiver, denom))
		require.Equal(t, coin.Amount, balance(from.Chain, escrow, coin.Denom))
		require.Equal(t, initial.Sub(coin.Amount), balance(from.Chain, sender, coin.Denom))
		back = sendIBC(t, to, voucher, sender.String(), uint64(coord.CurrentTime.Add(time.Hour).UnixNano()))
		require.NoError(t, path.RelayPacket(back))
		require.Equal(t, initial, balance(from.Chain, sender, coin.Denom))
		require.True(t, balance(from.Chain, escrow, coin.Denom).IsZero())
	})
}

func TestIBCRejectsTamperedPacket(t *testing.T) {
	coord, path := newIBCNetwork(t)
	from, to := path.EndpointA, path.EndpointB
	coin := sdk.NewInt64Coin(tokenomics.DefaultDenom, 1_000_000)
	receiver := to.Chain.SenderAccount.GetAddress()
	packet := sendIBC(t, from, coin, receiver.String(), uint64(coord.CurrentTime.Add(time.Hour).UnixNano()))
	require.NoError(t, to.UpdateClient())
	forged := packet
	forged.Data = append([]byte(nil), packet.Data...)
	forged.Data[0] ^= 1
	require.Error(t, to.RecvPacket(forged), "Merkle proof must reject a modified packet")
	denom := transfertypes.NewDenom(coin.Denom, transfertypes.NewHop(transfertypes.PortID, to.ChannelID)).IBCDenom()
	require.True(t, balance(to.Chain, receiver, denom).IsZero())
	require.NoError(t, path.RelayPacket(packet), "the original packet remains relayable")
	require.Equal(t, coin.Amount, balance(to.Chain, receiver, denom))
}

func TestIBCUpgradeRejectsUnsafeBaseline(t *testing.T) {
	_, path := newIBCNetwork(t)
	chain := path.EndpointA.Chain
	a := gridApp(chain)
	ctx := chain.GetContext()
	initial := balance(chain, chain.SenderAccount.GetAddress(), tokenomics.DefaultDenom)
	for _, name := range []string{"already enabled", "missing registry version", "old registry version"} {
		t.Run(name, func(t *testing.T) {
			vm := a.ModuleManager.GetVersionMap()
			if name != "already enabled" {
				delete(vm, "ibc")
				delete(vm, "transfer")
				delete(vm, "07-tendermint")
				if name == "missing registry version" {
					delete(vm, registry.ModuleName)
				} else {
					vm[registry.ModuleName]--
				}
			}
			require.NoError(t, a.UpgradeKeeper.SetModuleVersionMap(ctx, vm))
			err := a.UpgradeKeeper.ApplyUpgrade(ctx, upgradetypes.Plan{Name: app.IBCTransferV1UpgradeName, Height: ctx.BlockHeight()})
			require.Error(t, err)
			require.Equal(t, initial, balance(chain, chain.SenderAccount.GetAddress(), tokenomics.DefaultDenom))
			channel, found := a.IBCKeeper.ChannelKeeper.GetChannel(ctx, transfertypes.PortID, path.EndpointA.ChannelID)
			require.True(t, found, "failed upgrade must not reinitialize IBC state")
			require.Equal(t, channeltypes.OPEN, channel.State)
		})
	}
}
