package chainlink

import (
"math/big"
	"context"
	"encoding/json"

	"github.com/pkg/errors"

	starkchain "github.com/smartcontractkit/chainlink-starknet/relayer/pkg/chainlink/chain"
	"github.com/smartcontractkit/chainlink-starknet/relayer/pkg/chainlink/ocr2"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	relaytypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

var _ relaytypes.Relayer = (*relayer)(nil) //nolint:staticcheck

type relayer struct {
	chain starkchain.Chain

	lggr logger.Logger
}

func NewRelayer(lggr logger.Logger, chain starkchain.Chain) *relayer {
	return &relayer{
		chain:  chain,
		lggr:   logger.Named(lggr, "Relayer"),
	}
}

func (r *relayer) Name() string {
	return r.lggr.Name()
}

func (r *relayer) Start(context.Context) error {
	return nil
}

func (r *relayer) Close() error {	return nil}

func (r *relayer) Ready() error {
	return r.chain.Ready()
}

func (r *relayer) Healthy() error { return nil }

func (r *relayer) HealthReport() map[string]error {
	return map[string]error{r.Name(): r.Healthy()}
}

func (r *relayer) GetChainStatus(ctx context.Context) (relaytypes.ChainStatus, error) {
	return r.chain.GetChainStatus(ctx)
}

func (r *relayer) ListNodeStatuses(ctx context.Context, pageSize int32, pageToken string) (stats []relaytypes.NodeStatus, nextPageToken string, total int, err error) {
	return r.chain.ListNodeStatuses(ctx, pageSize, pageToken)
}

func (r *relayer) Transact(ctx context.Context, from, to string, amount *big.Int, balanceCheck bool) error {
	return r.chain.Transact(ctx, from, to, amount, balanceCheck)
}

func (r *relayer) NewConfigProvider(ctx context.Context, args relaytypes.RelayArgs) (relaytypes.ConfigProvider, error) {
	var relayConfig RelayConfig

	err := json.Unmarshal(args.RelayConfig, &relayConfig)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't unmarshal RelayConfig")
	}

	reader, err := r.chain.Reader()
	if err != nil {
		return nil, errors.Wrap(err, "error in NewConfigProvider chain.Reader")
	}
	configProvider, err := ocr2.NewConfigProvider(r.chain.ID(), args.ContractID, reader, r.chain.Config(), r.lggr)
	if err != nil {
		return nil, errors.Wrap(err, "coudln't initialize ConfigProvider")
	}

	return configProvider, nil
}

func (r *relayer) NewMedianProvider(ctx context.Context, rargs relaytypes.RelayArgs, pargs relaytypes.PluginArgs) (relaytypes.MedianProvider, error) {
	var relayConfig RelayConfig

	err := json.Unmarshal(rargs.RelayConfig, &relayConfig)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't unmarshal RelayConfig")
	}

	if relayConfig.AccountAddress == "" {
		return nil, errors.New("no account address in relay config")
	}

	// todo: use pargs for median provider
	reader, err := r.chain.Reader()
	if err != nil {
		return nil, errors.Wrap(err, "error in NewMedianProvider chain.Reader")
	}
	medianProvider, err := ocr2.NewMedianProvider(r.chain.ID(), rargs.ContractID, pargs.TransmitterID, relayConfig.AccountAddress, reader, r.chain.Config(), r.chain.TxManager(), r.lggr)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't initilize MedianProvider")
	}

	return medianProvider, nil
}

func (r *relayer) NewMercuryProvider(ctx context.Context, rargs relaytypes.RelayArgs, pargs relaytypes.PluginArgs) (relaytypes.MercuryProvider, error) {
	return nil, errors.New("mercury is not supported for starknet")
}

func (r *relayer) NewLLOProvider(ctx context.Context, rargs relaytypes.RelayArgs, pargs relaytypes.PluginArgs) (relaytypes.LLOProvider, error) {
	return nil, errors.New("data streams is not supported for starknet")
}

func (r *relayer) NewFunctionsProvider(ctx context.Context, rargs relaytypes.RelayArgs, pargs relaytypes.PluginArgs) (relaytypes.FunctionsProvider, error) {
	return nil, errors.New("functions are not supported for starknet")
}

func (r *relayer) NewAutomationProvider(ctx context.Context, rargs relaytypes.RelayArgs, pargs relaytypes.PluginArgs) (relaytypes.AutomationProvider, error) {
	return nil, errors.New("automation is not supported for starknet")
}
