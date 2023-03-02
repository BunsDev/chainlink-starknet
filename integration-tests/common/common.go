package common

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dontpanicdao/caigo/gateway"
	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/smartcontractkit/chainlink-env/environment"
	"github.com/smartcontractkit/chainlink-env/pkg/alias"
	"github.com/smartcontractkit/chainlink-env/pkg/helm/chainlink"
	"github.com/smartcontractkit/chainlink-env/pkg/helm/mockserver"
	mockservercfg "github.com/smartcontractkit/chainlink-env/pkg/helm/mockserver-cfg"
	"github.com/smartcontractkit/chainlink-starknet/ops/devnet"
	"github.com/smartcontractkit/chainlink/core/services/job"
	"github.com/smartcontractkit/chainlink/core/services/relay"
	"github.com/smartcontractkit/chainlink/integration-tests/client"
	"gopkg.in/guregu/null.v4"
)

var (
	serviceKeyL1        = "Hardhat"
	serviceKeyL2        = "starknet-dev"
	serviceKeyChainlink = "chainlink"
	chainName           = "starknet"
	chainId             = gateway.GOERLI_ID
)

type Common struct {
	P2PPort             string
	ServiceKeyL1        string
	ServiceKeyL2        string
	ServiceKeyChainlink string
	ChainName           string
	ChainId             string
	NodeCount           int
	TTL                 time.Duration
	Testnet             bool
	CLImage             string
	CLVersion           string
	L2RPCUrl            string
	PrivateKey          string
	Account             string
	ClConfig            map[string]interface{}
	EnvConfig           map[string]interface{}
	K8Config            *environment.Config
	Env                 *environment.Environment
}

func New() *Common {
	var err error
	c := &Common{
		ChainName:           chainName,
		ChainId:             chainId,
		ServiceKeyChainlink: serviceKeyChainlink,
		ServiceKeyL1:        serviceKeyL1,
		ServiceKeyL2:        serviceKeyL2,
	}
	// Checking if count of OCR nodes is defined in ENV
	nodeCountSet := getEnv("NODE_COUNT")
	if nodeCountSet != "" {
		c.NodeCount, err = strconv.Atoi(nodeCountSet)
		if err != nil {
			panic(fmt.Sprintf("Please define a proper node count for the test: %v", err))
		}
	} else {
		panic("Please define NODE_COUNT")
	}

	// Checking if TTL env var is set in ENV
	ttlValue := getEnv("TTL")
	if ttlValue != "" {
		duration, err := time.ParseDuration(ttlValue)
		if err != nil {
			panic(fmt.Sprintf("Please define a proper duration for the test: %v", err))
		}
		c.TTL, err = time.ParseDuration(*alias.ShortDur(duration))
		if err != nil {
			panic(fmt.Sprintf("Please define a proper duration for the test: %v", err))
		}
	} else {
		panic("Please define TTL of env")
	}

	// Checking if version needs to be overridden env var is set in ENV
	envClImage := getEnv("CL_IMAGE")
	envClVersion := getEnv("CL_VERSION")
	if envClImage != "" && envClVersion != "" {
		c.CLImage = envClImage
		c.CLVersion = envClVersion
	} else {
		panic("Please define CL_IMAGE and CL_VERSION")
	}

	// Setting optional parameters
	c.L2RPCUrl = getEnv("L2_RPC_URL") // Fetch L2 RPC url if defined
	c.Testnet = c.L2RPCUrl != ""
	c.PrivateKey = getEnv("PRIVATE_KEY")
	c.Account = getEnv("ACCOUNT")

	return c
}

// getEnv gets the environment variable if it exists and sets it for the remote runner
func getEnv(v string) string {
	val := os.Getenv(v)
	if val != "" {
		os.Setenv(fmt.Sprintf("TEST_%s", v), val)
	}
	return val
}

// CreateKeys Creates node keys and defines chain and nodes for each node
func (c *Common) CreateKeys(env *environment.Environment) ([]client.NodeKeysBundle, []*client.Chainlink, error) {
	ChainlinkNodes, err := client.ConnectChainlinkNodes(env)
	if err != nil {
		return nil, nil, err
	}
	NKeys, _, err := client.CreateNodeKeysBundle(ChainlinkNodes, c.ChainName, c.ChainId)
	if err != nil {
		return nil, nil, err
	}
	for _, n := range ChainlinkNodes {
		_, _, err = n.CreateStarkNetChain(&client.StarkNetChainAttributes{
			Type:    c.ChainName,
			ChainID: c.ChainId,
			Config:  client.StarkNetChainConfig{},
		})
		if err != nil {
			return nil, nil, err
		}
		_, _, err = n.CreateStarkNetNode(&client.StarkNetNodeAttributes{
			Name:    c.ChainName,
			ChainID: c.ChainId,
			Url:     env.URLs[c.ServiceKeyL2][1],
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return NKeys, ChainlinkNodes, nil
}

// CreateJobsForContract Creates and sets up the boostrap jobs as well as OCR jobs
func (c *Common) CreateJobsForContract(cc *ChainlinkClient, observationSource string, juelsPerFeeCoinSource string, ocrControllerAddress string) error {
	// Define node[0] as bootstrap node
	cc.bootstrapPeers = []client.P2PData{
		{
			RemoteIP:   cc.ChainlinkNodes[0].RemoteIP(),
			RemotePort: c.P2PPort,
			PeerID:     cc.NKeys[0].PeerID,
		},
	}

	// Defining relay config
	relayConfig := job.JSONConfig{
		"nodeName": fmt.Sprintf("\"starknet-OCRv2-%s-%s\"", "node", uuid.NewV4().String()),
		"chainID":  fmt.Sprintf("\"%s\"", c.ChainId),
	}

	oracleSpec := job.OCR2OracleSpec{
		ContractID:                  ocrControllerAddress,
		Relay:                       relay.StarkNet,
		RelayConfig:                 relayConfig,
		ContractConfigConfirmations: 1, // don't wait for confirmation on devnet
	}
	// Setting up bootstrap node
	jobSpec := &client.OCR2TaskJobSpec{
		Name:           fmt.Sprintf("starknet-OCRv2-%s-%s", "bootstrap", uuid.NewV4().String()),
		JobType:        "bootstrap",
		OCR2OracleSpec: oracleSpec,
	}

	_, _, err := cc.ChainlinkNodes[0].CreateJob(jobSpec)
	if err != nil {
		return err
	}

	var p2pBootstrappers []string

	for i := range cc.bootstrapPeers {
		p2pBootstrappers = append(p2pBootstrappers, cc.bootstrapPeers[i].P2PV2Bootstrapper())
	}

	// Setting up job specs
	for nIdx, n := range cc.ChainlinkNodes {
		if nIdx == 0 {
			continue
		}
		_, err := n.CreateBridge(cc.bTypeAttr)
		if err != nil {
			return err
		}

		oracleSpec = job.OCR2OracleSpec{
			ContractID:                  ocrControllerAddress,
			Relay:                       relay.StarkNet,
			RelayConfig:                 relayConfig,
			PluginType:                  "median",
			OCRKeyBundleID:              null.StringFrom(cc.NKeys[nIdx].OCR2Key.Data.ID),
			TransmitterID:               null.StringFrom(cc.NKeys[nIdx].TXKey.Data.ID),
			P2PV2Bootstrappers:          pq.StringArray{strings.Join(p2pBootstrappers, ",")},
			ContractConfigConfirmations: 1, // don't wait for confirmation on devnet
			PluginConfig: job.JSONConfig{
				"juelsPerFeeCoinSource": juelsPerFeeCoinSource,
			},
		}

		jobSpec = &client.OCR2TaskJobSpec{
			Name:              fmt.Sprintf("starknet-OCRv2-%d-%s", nIdx, uuid.NewV4().String()),
			JobType:           "offchainreporting2",
			OCR2OracleSpec:    oracleSpec,
			ObservationSource: observationSource,
		}
		_, _, err = n.CreateJob(jobSpec)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Common) Default(t *testing.T) {
	c.K8Config = &environment.Config{NamespacePrefix: "chainlink-ocr-starknet", TTL: c.TTL, Test: t}
	c.EnvConfig = map[string]interface{}{
		"STARKNET_ENABLED":            "true",
		"EVM_ENABLED":                 "false",
		"EVM_RPC_ENABLED":             "false",
		"CHAINLINK_DEV":               "false",
		"FEATURE_OFFCHAIN_REPORTING2": "true",
		"feature_offchain_reporting":  "false",
		"P2P_NETWORKING_STACK":        "V2",
		"P2PV2_LISTEN_ADDRESSES":      "0.0.0.0:6690",
		"P2PV2_DELTA_DIAL":            "5s",
		"P2PV2_DELTA_RECONCILE":       "5s",
		"p2p_listen_port":             "0",
	}
	c.ClConfig = map[string]interface{}{
		"replicas": c.NodeCount,
		"env":      c.EnvConfig,
		"chainlink": map[string]interface{}{
			"image": map[string]interface{}{
				"image":   c.CLImage,
				"version": c.CLVersion,
			},
		},
	}
	c.Env = environment.New(c.K8Config).
		AddHelm(devnet.New("0.0.11", nil)).
		AddHelm(mockservercfg.New(nil)).
		AddHelm(mockserver.New(nil)).
		AddHelm(chainlink.NewVersioned(0, "0.0.11", c.ClConfig))
}
