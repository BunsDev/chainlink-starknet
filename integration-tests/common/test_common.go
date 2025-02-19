package common

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/NethermindEth/juno/core/felt"
	starknetdevnet "github.com/NethermindEth/starknet.go/devnet"
	starknetutils "github.com/NethermindEth/starknet.go/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-starknet/relayer/pkg/chainlink/ocr2"
	"github.com/smartcontractkit/chainlink-starknet/relayer/pkg/starknet"

	"github.com/smartcontractkit/chainlink-starknet/ops"
	"github.com/smartcontractkit/chainlink-starknet/ops/devnet"
	"github.com/smartcontractkit/chainlink-starknet/ops/gauntlet"
	"github.com/smartcontractkit/chainlink/integration-tests/client"

	"github.com/smartcontractkit/chainlink-starknet/integration-tests/utils"
)

var (
	rpcRequestTimeout = time.Second * 300
	dumpPath          = "/dumps/dump.pkl"
)

type Test struct {
	Devnet                *devnet.StarknetDevnetClient
	Cc                    *ChainlinkClient
	Starknet              *starknet.Client
	OCR2Client            *ocr2.Client
	Sg                    *gauntlet.StarknetGauntlet
	L1RPCUrl              string
	Common                *Common
	AccountAddresses      []string
	LinkTokenAddr         string
	OCRAddr               string
	AccessControllerAddr  string
	ProxyAddr             string
	ObservationSource     string
	JuelsPerFeeCoinSource string
	T                     *testing.T
}

type ChainlinkClient struct {
	NKeys          []client.NodeKeysBundle
	ChainlinkNodes []*client.ChainlinkClient
	bTypeAttr      *client.BridgeTypeAttributes
	bootstrapPeers []client.P2PData
}

// DeployCluster Deploys and sets up config of the environment and nodes
func (testState *Test) DeployCluster() {
	lggr := logger.Nop()
	testState.Cc = &ChainlinkClient{}
	testState.ObservationSource = testState.GetDefaultObservationSource()
	testState.JuelsPerFeeCoinSource = testState.GetDefaultJuelsPerFeeCoinSource()
	testState.DeployEnv()
	if testState.Common.Env.WillUseRemoteRunner() {
		return // short circuit here if using a remote runner
	}
	testState.SetupClients()
	if testState.Common.Testnet {
		testState.Common.Env.URLs[testState.Common.ServiceKeyL2][1] = testState.Common.L2RPCUrl
	}
	var err error
	testState.Cc.NKeys, testState.Cc.ChainlinkNodes, err = testState.Common.CreateKeys(testState.Common.Env)
	require.NoError(testState.T, err, "Creating chains and keys should not fail") // TODO; fails here
	baseURL := testState.Common.L2RPCUrl
	if !testState.Common.Testnet { // devnet!
		// chainlink starknet client needs the RPC API url which is at /rpc on devnet
		baseURL += "/rpc"
	}
	testState.Starknet, err = starknet.NewClient(testState.Common.ChainId, baseURL, lggr, &rpcRequestTimeout)
	require.NoError(testState.T, err, "Creating starknet client should not fail")
	testState.OCR2Client, err = ocr2.NewClient(testState.Starknet, lggr)
	require.NoError(testState.T, err, "Creating ocr2 client should not fail")
	if !testState.Common.Testnet {
		// fetch predeployed account 0 to use as funder
		devnet := starknetdevnet.NewDevNet(testState.Common.L2RPCUrl)
		accounts, err := devnet.Accounts()
		require.NoError(testState.T, err)
		account := accounts[0]

		err = os.Setenv("PRIVATE_KEY", account.PrivateKey)
		require.NoError(testState.T, err, "Setting private key should not fail")
		err = os.Setenv("ACCOUNT", account.Address)
		require.NoError(testState.T, err, "Setting account address should not fail")
		testState.Devnet.AutoDumpState() // Auto dumping devnet state to avoid losing contracts on crash
	}
}

// DeployEnv Deploys the environment
func (testState *Test) DeployEnv() {
	testState.Common.SetLocalEnvironment(testState.T)
	// if testState.Common.Env.WillUseRemoteRunner() {
	// 	return // short circuit here if using a remote runner
	// }
}

// SetupClients Sets up the starknet client
func (testState *Test) SetupClients() {
	l := utils.GetTestLogger(testState.T)
	if testState.Common.Testnet {
		l.Debug().Msg(fmt.Sprintf("Overriding L2 RPC: %s", testState.Common.L2RPCUrl))
	} else {
		// TODO: HAXX:
		// testState.Common.L2RPCUrl = testState.Common.Env.URLs[testState.Common.ServiceKeyL2][0] // For local runs setting local ip
		// if testState.Common.Env.Cfg.InsideK8s {
		// 	testState.Common.L2RPCUrl = testState.Common.Env.URLs[testState.Common.ServiceKeyL2][1] // For remote runner setting remote IP
		// }
		l.Debug().Msg(fmt.Sprintf("L2 RPC: %s", testState.Common.L2RPCUrl))
		testState.Devnet = testState.Devnet.NewStarknetDevnetClient(testState.Common.L2RPCUrl, dumpPath)
	}
}

// LoadOCR2Config Loads and returns the default starknet gauntlet config
func (testState *Test) LoadOCR2Config() (*ops.OCR2Config, error) {
	var offChaiNKeys []string
	var onChaiNKeys []string
	var peerIds []string
	var txKeys []string
	var cfgKeys []string
	for i, key := range testState.Cc.NKeys {
		offChaiNKeys = append(offChaiNKeys, key.OCR2Key.Data.Attributes.OffChainPublicKey)
		peerIds = append(peerIds, key.PeerID)
		txKeys = append(txKeys, testState.AccountAddresses[i])
		onChaiNKeys = append(onChaiNKeys, key.OCR2Key.Data.Attributes.OnChainPublicKey)
		cfgKeys = append(cfgKeys, key.OCR2Key.Data.Attributes.ConfigPublicKey)
	}

	var payload = ops.TestOCR2Config
	payload.Signers = onChaiNKeys
	payload.Transmitters = txKeys
	payload.OffchainConfig.OffchainPublicKeys = offChaiNKeys
	payload.OffchainConfig.PeerIds = peerIds
	payload.OffchainConfig.ConfigPublicKeys = cfgKeys

	return &payload, nil
}

func (testState *Test) SetUpNodes() {
	err := testState.Common.CreateJobsForContract(testState.GetChainlinkClient(), testState.Common.MockUrl, testState.ObservationSource, testState.JuelsPerFeeCoinSource, testState.OCRAddr, testState.AccountAddresses)
	require.NoError(testState.T, err, "Creating jobs should not fail")
}

// GetNodeKeys Returns the node key bundles
func (testState *Test) GetNodeKeys() []client.NodeKeysBundle {
	return testState.Cc.NKeys
}

func (testState *Test) GetChainlinkNodes() []*client.ChainlinkClient {
	return testState.Cc.ChainlinkNodes
}

func (testState *Test) GetChainlinkClient() *ChainlinkClient {
	return testState.Cc
}

func (testState *Test) GetStarknetDevnetClient() *devnet.StarknetDevnetClient {
	return testState.Devnet
}

func (testState *Test) SetBridgeTypeAttrs(attr *client.BridgeTypeAttributes) {
	testState.Cc.bTypeAttr = attr
}

// ConfigureL1Messaging Sets the L1 messaging contract location and RPC url on L2
func (testState *Test) ConfigureL1Messaging() {
	err := testState.Devnet.LoadL1MessagingContract(testState.L1RPCUrl)
	require.NoError(testState.T, err, "Setting up L1 messaging should not fail")
}

func (testState *Test) GetDefaultObservationSource() string {
	return `
			val [type = "bridge" name="mockserver-bridge"]
			parse [type="jsonparse" path="data,result"]
			val -> parse
			`
}

func (testState *Test) GetDefaultJuelsPerFeeCoinSource() string {
	return `"""
			sum  [type="sum" values=<[451000]> ]
			sum
			"""
			`
}

func (testState *Test) ValidateRounds(rounds int, isSoak bool) error {
	l := utils.GetTestLogger(testState.T)
	ctx := context.Background() // context background used because timeout handled by requestTimeout param
	// assert new rounds are occurring
	details := ocr2.TransmissionDetails{}
	increasing := 0 // track number of increasing rounds
	var stuck bool
	stuckCount := 0
	var positive bool

	// validate balance in aggregator
	linkContractAddress, err := starknetutils.HexToFelt(testState.LinkTokenAddr)
	if err != nil {
		return err
	}
	contractAddress, err := starknetutils.HexToFelt(testState.OCRAddr)
	if err != nil {
		return err
	}
	resLINK, errLINK := testState.Starknet.CallContract(ctx, starknet.CallOps{
		ContractAddress: linkContractAddress,
		Selector:        starknetutils.GetSelectorFromNameFelt("balance_of"),
		Calldata:        []*felt.Felt{contractAddress},
	})
	require.NoError(testState.T, errLINK, "Reader balance from LINK contract should not fail", "err", errLINK)
	resAgg, errAgg := testState.Starknet.CallContract(ctx, starknet.CallOps{
		ContractAddress: contractAddress,
		Selector:        starknetutils.GetSelectorFromNameFelt("link_available_for_payment"),
	})
	require.NoError(testState.T, errAgg, "link_available_for_payment should not fail", "err", errAgg)
	balLINK := resLINK[0].BigInt(big.NewInt(0))
	balAgg := resAgg[1].BigInt(big.NewInt(0))
	isNegative := resAgg[0].BigInt(big.NewInt(0))
	if isNegative.Sign() > 0 {
		balAgg = new(big.Int).Neg(balAgg)
	}

	assert.Equal(testState.T, balLINK.Cmp(big.NewInt(0)), 1, "Aggregator should have non-zero balance")
	assert.GreaterOrEqual(testState.T, balLINK.Cmp(balAgg), 0, "Aggregator payment balance should be <= actual LINK balance")

	for start := time.Now(); time.Since(start) < testState.Common.TestDuration; {
		l.Info().Msg(fmt.Sprintf("Elapsed time: %s, Round wait: %s ", time.Since(start), testState.Common.TestDuration))
		res, err2 := testState.OCR2Client.LatestTransmissionDetails(ctx, contractAddress)
		require.NoError(testState.T, err2, "Failed to get latest transmission details")
		// end condition: enough rounds have occurred
		if !isSoak && increasing >= rounds && positive {
			break
		}

		// end condition: rounds have been stuck
		if stuck && stuckCount > 50 {
			l.Debug().Msg("failing to fetch transmissions means blockchain may have stopped")
			break
		}

		// try to fetch rounds
		time.Sleep(5 * time.Second)

		if err != nil {
			l.Error().Msg(fmt.Sprintf("Transmission Error: %+v", err))
			continue
		}
		l.Info().Msg(fmt.Sprintf("Transmission Details: %+v", res))

		// continue if no changes
		if res.Epoch == 0 && res.Round == 0 {
			continue
		}

		ansCmp := res.LatestAnswer.Cmp(big.NewInt(0))
		positive = ansCmp == 1 || positive

		// if changes from zero values set (should only initially)
		if res.Epoch > 0 && details.Epoch == 0 {
			if !isSoak {
				assert.Greater(testState.T, res.Epoch, details.Epoch)
				assert.GreaterOrEqual(testState.T, res.Round, details.Round)
				assert.NotEqual(testState.T, ansCmp, 0) // assert changed from 0
				assert.NotEqual(testState.T, res.Digest, details.Digest)
				assert.Equal(testState.T, details.LatestTimestamp.Before(res.LatestTimestamp), true)
			}
			details = res
			continue
		}
		// check increasing rounds
		if !isSoak {
			assert.Equal(testState.T, res.Digest, details.Digest, "Config digest should not change")
		} else {
			if res.Digest != details.Digest {
				l.Error().Msg(fmt.Sprintf("Config digest should not change, expected %s got %s", details.Digest, res.Digest))
			}
		}
		if (res.Epoch > details.Epoch || (res.Epoch == details.Epoch && res.Round > details.Round)) && details.LatestTimestamp.Before(res.LatestTimestamp) {
			increasing++
			stuck = false
			stuckCount = 0 // reset counter
			continue
		}

		// reach this point, answer has not changed
		stuckCount++
		if stuckCount > 30 {
			stuck = true
			increasing = 0
		}
	}
	if !isSoak {
		assert.GreaterOrEqual(testState.T, increasing, rounds, "Round + epochs should be increasing")
		assert.Equal(testState.T, positive, true, "Positive value should have been submitted")
		assert.Equal(testState.T, stuck, false, "Round + epochs should not be stuck")
	}

	// Test proxy reading
	// TODO: would be good to test proxy switching underlying feeds

	proxyAddress, err := starknetutils.HexToFelt(testState.ProxyAddr)
	if err != nil {
		return err
	}
	roundDataRaw, err := testState.Starknet.CallContract(ctx, starknet.CallOps{
		ContractAddress: proxyAddress,
		Selector:        starknetutils.GetSelectorFromNameFelt("latest_round_data"),
	})
	if !isSoak {
		require.NoError(testState.T, err, "Reading round data from proxy should not fail")
		assert.Equal(testState.T, len(roundDataRaw), 5, "Round data from proxy should match expected size")
	}
	valueBig := roundDataRaw[1].BigInt(big.NewInt(0))
	require.NoError(testState.T, err)
	value := valueBig.Int64()
	if value < 0 {
		assert.Equal(testState.T, value, int64(5), "Reading from proxy should return correct value")
	}

	return nil
}
