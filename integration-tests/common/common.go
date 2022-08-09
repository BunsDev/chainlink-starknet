package common

import (
	"fmt"
	uuid "github.com/satori/go.uuid"
	"github.com/smartcontractkit/chainlink-env/environment"
	ctfClient "github.com/smartcontractkit/chainlink-testing-framework/client"
	"github.com/smartcontractkit/chainlink/integration-tests/client"
)

const (
	serviceKeyL1        = "Hardhat"
	serviceKeyL2        = "starknet-dev"
	serviceKeyChainlink = "chainlink"
	chainName           = "starknet"
	chainId             = "devnet"
)

var (
	defaultP2PPort = "8090"
)

// CreateKeys Creates node keys and defines chain and nodes for each node
func CreateKeys(env *environment.Environment) ([]ctfClient.NodeKeysBundle, []*client.Chainlink, error) {
	chainlinkNodes, err := client.ConnectChainlinkNodes(env)
	if err != nil {
		return nil, nil, err
	}
	nKeys, err := ctfClient.CreateNodeKeysBundle(chainlinkNodes, chainName)
	if err != nil {
		return nil, nil, err
	}
	for _, n := range chainlinkNodes {
		_, _, err = n.CreateStarknetChain(&client.StarknetChainAttributes{
			Type:    chainName,
			ChainID: chainId,
			Config:  client.StarknetChainConfig{},
		})
		if err != nil {
			return nil, nil, err
		}
		_, _, err = n.CreateStarknetNode(&client.StarknetNodeAttributes{
			Name:    chainName,
			ChainID: chainId,
			Url:     env.URLs[serviceKeyL2][1],
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return nKeys, chainlinkNodes, nil
}

// CreateJobsForContract Creates and sets up the boostrap jobs as well as OCR jobs
func CreateJobsForContract(cc *ChainlinkClient, juelsPerFeeCoinSource string, ocrControllerAddress string) error {
	// Defining bootstrap peers
	for nIdx, n := range cc.chainlinkNodes {
		cc.bootstrapPeers = append(cc.bootstrapPeers, client.P2PData{
			RemoteIP:   n.RemoteIP(),
			RemotePort: defaultP2PPort,
			PeerID:     cc.nKeys[nIdx].PeerID,
		})
	}
	// Defining relay config
	relayConfig := map[string]string{
		"nodeName": fmt.Sprintf("starknet-OCRv2-%s-%s", "node", uuid.NewV4().String()),
		"chainID":  chainId,
	}

	// Setting up bootstrap node
	jobSpec := &client.OCR2TaskJobSpec{
		Name:               fmt.Sprintf("starknet-OCRv2-%s-%s", "bootstrap", uuid.NewV4().String()),
		JobType:            "bootstrap",
		ContractID:         ocrControllerAddress,
		Relay:              chainName,
		RelayConfig:        relayConfig,
		P2PV2Bootstrappers: cc.bootstrapPeers,
	}
	_, _, err := cc.chainlinkNodes[0].CreateJob(jobSpec)
	if err != nil {
		return err
	}

	// Setting up job specs
	for nIdx, n := range cc.chainlinkNodes {
		if nIdx == 0 {
			continue
		}
		_, err = n.CreateBridge(cc.bTypeAttr)
		if err != nil {
			return err
		}
		jobSpec := &client.OCR2TaskJobSpec{
			Name:                  fmt.Sprintf("starknet-OCRv2-%d-%s", nIdx, uuid.NewV4().String()),
			JobType:               "offchainreporting2",
			ContractID:            ocrControllerAddress,
			Relay:                 chainName,
			RelayConfig:           relayConfig,
			PluginType:            "median",
			P2PV2Bootstrappers:    cc.bootstrapPeers,
			OCRKeyBundleID:        cc.nKeys[nIdx].OCR2Key.Data.ID,
			TransmitterID:         cc.nKeys[nIdx].TXKey.Data.ID,
			ObservationSource:     client.ObservationSourceSpecBridge(*cc.bTypeAttr),
			JuelsPerFeeCoinSource: juelsPerFeeCoinSource,
		}
		_, _, err := n.CreateJob(jobSpec)
		if err != nil {
			return err
		}
	}
	return nil
}

func GetServiceKeyL1() string {
	return serviceKeyL1
}
func GetServiceKeyL2() string {
	return serviceKeyL2
}

func SetDefaultP2PPort(port string) {
	defaultP2PPort = port
}
