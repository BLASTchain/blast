package proposer

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	_ "net/http/pprof"
	"sync"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"

	"github.com/BLASTchain/blast/bl-bindings/bindings"
	"github.com/BLASTchain/blast/bl-proposer/flags"
	"github.com/BLASTchain/blast/bl-proposer/metrics"
	opservice "github.com/BLASTchain/blast/bl-service"
	"github.com/BLASTchain/blast/bl-service/dial"
	"github.com/BLASTchain/blast/bl-service/eth"
	oplog "github.com/BLASTchain/blast/bl-service/log"
	"github.com/BLASTchain/blast/bl-service/opio"
	oppprof "github.com/BLASTchain/blast/bl-service/pprof"
	oprpc "github.com/BLASTchain/blast/bl-service/rpc"
	"github.com/BLASTchain/blast/bl-service/sources"
	"github.com/BLASTchain/blast/bl-service/txmgr"
)

var supportedL2OutputVersion = eth.Bytes32{}

// Main is the entrypoint into the L2 Output Submitter. This method executes the
// service and blocks until the service exits.
func Main(version string, cliCtx *cli.Context) error {
	if err := flags.CheckRequired(cliCtx); err != nil {
		return err
	}
	cfg := NewConfig(cliCtx)
	if err := cfg.Check(); err != nil {
		return fmt.Errorf("invalid CLI flags: %w", err)
	}

	l := oplog.NewLogger(oplog.AppOut(cliCtx), cfg.LogConfig)
	oplog.SetGlobalLogHandler(l.GetHandler())
	opservice.ValidateEnvVars(flags.EnvVarPrefix, flags.Flags, l)
	m := metrics.NewMetrics("default")
	l.Info("Initializing L2 Output Submitter")

	proposerConfig, err := NewL2OutputSubmitterConfigFromCLIConfig(cfg, l, m)
	if err != nil {
		l.Error("Unable to create the L2 Output Submitter", "error", err)
		return err
	}

	l2OutputSubmitter, err := NewL2OutputSubmitter(*proposerConfig, l, m)
	if err != nil {
		l.Error("Unable to create the L2 Output Submitter", "error", err)
		return err
	}

	l.Info("Starting L2 Output Submitter")
	if err := l2OutputSubmitter.Start(); err != nil {
		l.Error("Unable to start L2 Output Submitter", "error", err)
		return err
	}
	defer l2OutputSubmitter.Stop()

	l.Info("L2 Output Submitter started")
	pprofConfig := cfg.PprofConfig
	if pprofConfig.Enabled {
		l.Debug("starting pprof", "addr", pprofConfig.ListenAddr, "port", pprofConfig.ListenPort)
		pprofSrv, err := oppprof.StartServer(pprofConfig.ListenAddr, pprofConfig.ListenPort)
		if err != nil {
			l.Error("failed to start pprof server", "err", err)
			return err
		}
		l.Info("started pprof server", "addr", pprofSrv.Addr())
		defer func() {
			if err := pprofSrv.Stop(context.Background()); err != nil {
				l.Error("failed to stop pprof server", "err", err)
			}
		}()
	}

	metricsCfg := cfg.MetricsConfig
	if metricsCfg.Enabled {
		l.Debug("starting metrics server", "addr", metricsCfg.ListenAddr, "port", metricsCfg.ListenPort)
		metricsSrv, err := m.Start(metricsCfg.ListenAddr, metricsCfg.ListenPort)
		if err != nil {
			return fmt.Errorf("failed to start metrics server: %w", err)
		}
		l.Info("started metrics server", "addr", metricsSrv.Addr())
		defer func() {
			if err := metricsSrv.Stop(context.Background()); err != nil {
				l.Error("failed to stop metrics server", "err", err)
			}
		}()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		m.StartBalanceMetrics(ctx, l, proposerConfig.L1Client, proposerConfig.TxManager.From())
	}

	rpcCfg := cfg.RPCConfig
	server := oprpc.NewServer(rpcCfg.ListenAddr, rpcCfg.ListenPort, version, oprpc.WithLogger(l))
	if rpcCfg.EnableAdmin {
		server.AddAPI(oprpc.ToGethAdminAPI(oprpc.NewCommonAdminAPI(&m.RPCMetrics, l)))
		l.Info("Admin RPC enabled")
	}
	if err := server.Start(); err != nil {
		return fmt.Errorf("error starting RPC server: %w", err)
	}

	m.RecordInfo(version)
	m.RecordUp()

	opio.BlockOnInterrupts()

	return nil
}

// L2OutputSubmitter is responsible for proposing outputs
type L2OutputSubmitter struct {
	txMgr txmgr.TxManager
	wg    sync.WaitGroup
	done  chan struct{}
	log   log.Logger
	metr  metrics.Metricer

	ctx    context.Context
	cancel context.CancelFunc

	// RollupClient is used to retrieve output roots from
	rollupClient *sources.RollupClient

	l2ooContract     *bindings.L2OutputOracleCaller
	l2ooContractAddr common.Address
	l2ooABI          *abi.ABI

	// AllowNonFinalized enables the proposal of safe, but non-finalized L2 blocks.
	// The L1 block-hash embedded in the proposal TX is checked and should ensure the proposal
	// is never valid on an alternative L1 chain that would produce different L2 data.
	// This option is not necessary when higher proposal latency is acceptable and L1 is healthy.
	allowNonFinalized bool
	// How frequently to poll L2 for new finalized outputs
	pollInterval   time.Duration
	networkTimeout time.Duration
}

// NewL2OutputSubmitterFromCLIConfig creates a new L2 Output Submitter given the CLI Config
func NewL2OutputSubmitterFromCLIConfig(cfg CLIConfig, l log.Logger, m metrics.Metricer) (*L2OutputSubmitter, error) {
	proposerConfig, err := NewL2OutputSubmitterConfigFromCLIConfig(cfg, l, m)
	if err != nil {
		return nil, err
	}
	return NewL2OutputSubmitter(*proposerConfig, l, m)
}

// NewL2OutputSubmitterConfigFromCLIConfig creates the proposer config from the CLI config.
func NewL2OutputSubmitterConfigFromCLIConfig(cfg CLIConfig, l log.Logger, m metrics.Metricer) (*Config, error) {
	l2ooAddress, err := opservice.ParseAddress(cfg.L2OOAddress)
	if err != nil {
		return nil, err
	}

	txManager, err := txmgr.NewSimpleTxManager("proposer", l, m, cfg.TxMgrConfig)
	if err != nil {
		return nil, err
	}

	// Connect to L1 and L2 providers. Perform these last since they are the most expensive.
	l1Client, err := dial.DialEthClientWithTimeout(context.Background(), dial.DefaultDialTimeout, l, cfg.L1EthRpc)
	if err != nil {
		return nil, err
	}

	rollupClient, err := dial.DialRollupClientWithTimeout(context.Background(), dial.DefaultDialTimeout, l, cfg.RollupRpc)
	if err != nil {
		return nil, err
	}

	return &Config{
		L2OutputOracleAddr: l2ooAddress,
		PollInterval:       cfg.PollInterval,
		NetworkTimeout:     cfg.TxMgrConfig.NetworkTimeout,
		L1Client:           l1Client,
		RollupClient:       rollupClient,
		AllowNonFinalized:  cfg.AllowNonFinalized,
		TxManager:          txManager,
	}, nil

}

// NewL2OutputSubmitter creates a new L2 Output Submitter
func NewL2OutputSubmitter(cfg Config, l log.Logger, m metrics.Metricer) (*L2OutputSubmitter, error) {
	ctx, cancel := context.WithCancel(context.Background())

	l2ooContract, err := bindings.NewL2OutputOracleCaller(cfg.L2OutputOracleAddr, cfg.L1Client)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create L2OO at address %s: %w", cfg.L2OutputOracleAddr, err)
	}

	cCtx, cCancel := context.WithTimeout(ctx, cfg.NetworkTimeout)
	defer cCancel()
	version, err := l2ooContract.Version(&bind.CallOpts{Context: cCtx})
	if err != nil {
		cancel()
		return nil, err
	}
	log.Info("Connected to L2OutputOracle", "address", cfg.L2OutputOracleAddr, "version", version)

	parsed, err := bindings.L2OutputOracleMetaData.GetAbi()
	if err != nil {
		cancel()
		return nil, err
	}

	return &L2OutputSubmitter{
		txMgr:  cfg.TxManager,
		done:   make(chan struct{}),
		log:    l,
		ctx:    ctx,
		cancel: cancel,
		metr:   m,

		rollupClient: cfg.RollupClient,

		l2ooContract:     l2ooContract,
		l2ooContractAddr: cfg.L2OutputOracleAddr,
		l2ooABI:          parsed,

		allowNonFinalized: cfg.AllowNonFinalized,
		pollInterval:      cfg.PollInterval,
		networkTimeout:    cfg.NetworkTimeout,
	}, nil
}

func (l *L2OutputSubmitter) Start() error {
	l.wg.Add(1)
	go l.loop()
	return nil
}

func (l *L2OutputSubmitter) Stop() {
	l.cancel()
	close(l.done)
	l.wg.Wait()
}

// FetchNextOutputInfo gets the block number of the next proposal.
// It returns: the next block number, if the proposal should be made, error
func (l *L2OutputSubmitter) FetchNextOutputInfo(ctx context.Context) (*eth.OutputResponse, bool, error) {
	cCtx, cancel := context.WithTimeout(ctx, l.networkTimeout)
	defer cancel()
	callOpts := &bind.CallOpts{
		From:    l.txMgr.From(),
		Context: cCtx,
	}
	nextCheckpointBlock, err := l.l2ooContract.NextBlockNumber(callOpts)
	if err != nil {
		l.log.Error("proposer unable to get next block number", "err", err)
		return nil, false, err
	}
	// Fetch the current L2 heads
	cCtx, cancel = context.WithTimeout(ctx, l.networkTimeout)
	defer cancel()
	status, err := l.rollupClient.SyncStatus(cCtx)
	if err != nil {
		l.log.Error("proposer unable to get sync status", "err", err)
		return nil, false, err
	}

	// Use either the finalized or safe head depending on the config. Finalized head is default & safer.
	var currentBlockNumber *big.Int
	if l.allowNonFinalized {
		currentBlockNumber = new(big.Int).SetUint64(status.SafeL2.Number)
	} else {
		currentBlockNumber = new(big.Int).SetUint64(status.FinalizedL2.Number)
	}
	// Ensure that we do not submit a block in the future
	if currentBlockNumber.Cmp(nextCheckpointBlock) < 0 {
		l.log.Debug("proposer submission interval has not elapsed", "currentBlockNumber", currentBlockNumber, "nextBlockNumber", nextCheckpointBlock)
		return nil, false, nil
	}

	return l.fetchOutput(ctx, nextCheckpointBlock)
}

func (l *L2OutputSubmitter) fetchOutput(ctx context.Context, block *big.Int) (*eth.OutputResponse, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, l.networkTimeout)
	defer cancel()
	output, err := l.rollupClient.OutputAtBlock(ctx, block.Uint64())
	if err != nil {
		l.log.Error("failed to fetch output at block %d: %w", block, err)
		return nil, false, err
	}
	if output.Version != supportedL2OutputVersion {
		l.log.Error("unsupported l2 output version: %s", output.Version)
		return nil, false, errors.New("unsupported l2 output version")
	}
	if output.BlockRef.Number != block.Uint64() { // sanity check, e.g. in case of bad RPC caching
		l.log.Error("invalid blockNumber: next blockNumber is %v, blockNumber of block is %v", block, output.BlockRef.Number)
		return nil, false, errors.New("invalid blockNumber")
	}

	// Always propose if it's part of the Finalized L2 chain. Or if allowed, if it's part of the safe L2 chain.
	if !(output.BlockRef.Number <= output.Status.FinalizedL2.Number || (l.allowNonFinalized && output.BlockRef.Number <= output.Status.SafeL2.Number)) {
		l.log.Debug("not proposing yet, L2 block is not ready for proposal",
			"l2_proposal", output.BlockRef,
			"l2_safe", output.Status.SafeL2,
			"l2_finalized", output.Status.FinalizedL2,
			"allow_non_finalized", l.allowNonFinalized)
		return nil, false, nil
	}
	return output, true, nil
}

// ProposeL2OutputTxData creates the transaction data for the ProposeL2Output function
func (l *L2OutputSubmitter) ProposeL2OutputTxData(output *eth.OutputResponse) ([]byte, error) {
	return proposeL2OutputTxData(l.l2ooABI, output)
}

// proposeL2OutputTxData creates the transaction data for the ProposeL2Output function
func proposeL2OutputTxData(abi *abi.ABI, output *eth.OutputResponse) ([]byte, error) {
	return abi.Pack(
		"proposeL2Output",
		output.OutputRoot,
		new(big.Int).SetUint64(output.BlockRef.Number),
		output.Status.CurrentL1.Hash,
		new(big.Int).SetUint64(output.Status.CurrentL1.Number))
}

// We wait until l1head advances beyond blocknum. This is used to make sure proposal tx won't
// immediately fail when checking the l1 blockhash. Note that EstimateGas uses "latest" state to
// execute the transaction by default, meaning inside the call, the head block is considered
// "pending" instead of committed. In the case l1blocknum == l1head then, blockhash(l1blocknum)
// will produce a value of 0 within EstimateGas, and the call will fail when the contract checks
// that l1blockhash matches blockhash(l1blocknum).
func (l *L2OutputSubmitter) waitForL1Head(ctx context.Context, blockNum uint64) error {
	ticker := time.NewTicker(l.pollInterval)
	defer ticker.Stop()
	l1head, err := l.txMgr.BlockNumber(ctx)
	if err != nil {
		return err
	}
	for l1head <= blockNum {
		l.log.Debug("waiting for l1 head > l1blocknum1+1", "l1head", l1head, "l1blocknum", blockNum)
		select {
		case <-ticker.C:
			l1head, err = l.txMgr.BlockNumber(ctx)
			if err != nil {
				return err
			}
			break
		case <-l.done:
			return fmt.Errorf("L2OutputSubmitter is done()")
		}
	}
	return nil
}

// sendTransaction creates & sends transactions through the underlying transaction manager.
func (l *L2OutputSubmitter) sendTransaction(ctx context.Context, output *eth.OutputResponse) error {
	err := l.waitForL1Head(ctx, output.Status.HeadL1.Number+1)
	if err != nil {
		return err
	}
	data, err := l.ProposeL2OutputTxData(output)
	if err != nil {
		return err
	}
	receipt, err := l.txMgr.Send(ctx, txmgr.TxCandidate{
		TxData:   data,
		To:       &l.l2ooContractAddr,
		GasLimit: 0,
	})
	if err != nil {
		return err
	}
	if receipt.Status == types.ReceiptStatusFailed {
		l.log.Error("proposer tx successfully published but reverted", "tx_hash", receipt.TxHash)
	} else {
		l.log.Info("proposer tx successfully published",
			"tx_hash", receipt.TxHash,
			"l1blocknum", output.Status.CurrentL1.Number,
			"l1blockhash", output.Status.CurrentL1.Hash)
	}
	return nil
}

// loop is responsible for creating & submitting the next outputs
func (l *L2OutputSubmitter) loop() {
	defer l.wg.Done()

	ctx := l.ctx

	ticker := time.NewTicker(l.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			output, shouldPropose, err := l.FetchNextOutputInfo(ctx)
			if err != nil {
				break
			}
			if !shouldPropose {
				break
			}
			cCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			if err := l.sendTransaction(cCtx, output); err != nil {
				l.log.Error("Failed to send proposal transaction",
					"err", err,
					"l1blocknum", output.Status.CurrentL1.Number,
					"l1blockhash", output.Status.CurrentL1.Hash,
					"l1head", output.Status.HeadL1.Number)
				cancel()
				break
			}
			l.metr.RecordL2BlocksProposed(output.BlockRef)
			cancel()

		case <-l.done:
			return
		}
	}
}
