package challenger

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	op_challenger "github.com/BLASTchain/blast/bl-challenger"
	"github.com/BLASTchain/blast/bl-challenger/config"
	"github.com/BLASTchain/blast/bl-e2e/e2eutils"
	"github.com/BLASTchain/blast/bl-e2e/e2eutils/wait"
	"github.com/BLASTchain/blast/bl-node/rollup"
	"github.com/BLASTchain/blast/bl-service/testlog"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/log"
	"github.com/stretchr/testify/require"
)

type Helper struct {
	log     log.Logger
	t       *testing.T
	require *require.Assertions
	dir     string
	cancel  func()
	errors  chan error
}

type Option func(config2 *config.Config)

func WithFactoryAddress(addr common.Address) Option {
	return func(c *config.Config) {
		c.GameFactoryAddress = addr
	}
}

func WithGameAddress(addr common.Address) Option {
	return func(c *config.Config) {
		c.GameAllowlist = append(c.GameAllowlist, addr)
	}
}

func WithPrivKey(key *ecdsa.PrivateKey) Option {
	return func(c *config.Config) {
		c.TxMgrConfig.PrivateKey = e2eutils.EncodePrivKeyToString(key)
	}
}

func WithAgreeProposedOutput(agree bool) Option {
	return func(c *config.Config) {
		c.AgreeWithProposedOutput = agree
	}
}

func WithAlphabet(alphabet string) Option {
	return func(c *config.Config) {
		c.TraceTypes = append(c.TraceTypes, config.TraceTypeAlphabet)
		c.AlphabetTrace = alphabet
	}
}

func WithPollInterval(pollInterval time.Duration) Option {
	return func(c *config.Config) {
		c.PollInterval = pollInterval
	}
}

func WithCannon(
	t *testing.T,
	rollupCfg *rollup.Config,
	l2Genesis *core.Genesis,
	l2Endpoint string,
) Option {
	return func(c *config.Config) {
		c.TraceTypes = append(c.TraceTypes, config.TraceTypeCannon)
		applyCannonConfig(c, t, rollupCfg, l2Genesis, l2Endpoint)
	}
}

func applyCannonConfig(
	c *config.Config,
	t *testing.T,
	rollupCfg *rollup.Config,
	l2Genesis *core.Genesis,
	l2Endpoint string,
) {
	require := require.New(t)
	c.CannonL2 = l2Endpoint
	c.CannonBin = "../cannon/bin/cannon"
	c.CannonServer = "../bl-program/bin/bl-program"
	c.CannonAbsolutePreState = "../bl-program/bin/prestate.json"
	c.CannonSnapshotFreq = 10_000_000

	genesisBytes, err := json.Marshal(l2Genesis)
	require.NoError(err, "marshall l2 genesis config")
	genesisFile := filepath.Join(c.Datadir, "l2-genesis.json")
	require.NoError(os.WriteFile(genesisFile, genesisBytes, 0644))
	c.CannonL2GenesisPath = genesisFile

	rollupBytes, err := json.Marshal(rollupCfg)
	require.NoError(err, "marshall rollup config")
	rollupFile := filepath.Join(c.Datadir, "rollup.json")
	require.NoError(os.WriteFile(rollupFile, rollupBytes, 0644))
	c.CannonRollupConfigPath = rollupFile
}

func WithOutputCannon(
	t *testing.T,
	rollupCfg *rollup.Config,
	l2Genesis *core.Genesis,
	rollupEndpoint string,
	l2Endpoint string) Option {
	return func(c *config.Config) {
		c.TraceTypes = append(c.TraceTypes, config.TraceTypeOutputCannon)
		c.RollupRpc = rollupEndpoint
		applyCannonConfig(c, t, rollupCfg, l2Genesis, l2Endpoint)
	}
}

func NewChallenger(t *testing.T, ctx context.Context, l1Endpoint string, name string, options ...Option) *Helper {
	log := testlog.Logger(t, log.LvlDebug).New("role", name)
	log.Info("Creating challenger", "l1", l1Endpoint)
	cfg := NewChallengerConfig(t, l1Endpoint, options...)

	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer close(errCh)
		errCh <- op_challenger.Main(ctx, log, cfg)
	}()
	return &Helper{
		log:     log,
		t:       t,
		require: require.New(t),
		dir:     cfg.Datadir,
		cancel:  cancel,
		errors:  errCh,
	}
}

func NewChallengerConfig(t *testing.T, l1Endpoint string, options ...Option) *config.Config {
	// Use the NewConfig method to ensure we pick up any defaults that are set.
	cfg := config.NewConfig(common.Address{}, l1Endpoint, true, t.TempDir())
	cfg.TxMgrConfig.NumConfirmations = 1
	cfg.TxMgrConfig.ReceiptQueryInterval = 1 * time.Second
	if cfg.MaxConcurrency > 4 {
		// Limit concurrency to something more reasonable when there are also multiple tests executing in parallel
		cfg.MaxConcurrency = 4
	}
	for _, option := range options {
		option(&cfg)
	}
	require.NotEmpty(t, cfg.TxMgrConfig.PrivateKey, "Missing private key for TxMgrConfig")
	require.NoError(t, cfg.Check(), "bl-challenger config should be valid")

	if cfg.CannonBin != "" {
		_, err := os.Stat(cfg.CannonBin)
		require.NoError(t, err, "cannon should be built. Make sure you've run make cannon-prestate")
	}
	if cfg.CannonServer != "" {
		_, err := os.Stat(cfg.CannonServer)
		require.NoError(t, err, "bl-program should be built. Make sure you've run make cannon-prestate")
	}
	if cfg.CannonAbsolutePreState != "" {
		_, err := os.Stat(cfg.CannonAbsolutePreState)
		require.NoError(t, err, "cannon pre-state should be built. Make sure you've run make cannon-prestate")
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = time.Second
	}

	return &cfg
}

func (h *Helper) Close() error {
	h.cancel()
	select {
	case <-time.After(1 * time.Minute):
		return errors.New("timed out while stopping challenger")
	case err := <-h.errors:
		if !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	}
}

type GameAddr interface {
	Addr() common.Address
}

func (h *Helper) VerifyGameDataExists(games ...GameAddr) {
	for _, game := range games {
		addr := game.Addr()
		h.require.DirExistsf(h.gameDataDir(addr), "should have data for game %v", addr)
	}
}

func (h *Helper) WaitForGameDataDeletion(ctx context.Context, games ...GameAddr) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	err := wait.For(ctx, time.Second, func() (bool, error) {
		for _, game := range games {
			addr := game.Addr()
			dir := h.gameDataDir(addr)
			_, err := os.Stat(dir)
			if errors.Is(err, os.ErrNotExist) {
				// This game has been successfully deleted
				continue
			}
			if err != nil {
				return false, fmt.Errorf("failed to check dir %v is deleted: %w", dir, err)
			}
			h.t.Logf("Game data directory %v not yet deleted", dir)
			return false, nil
		}
		return true, nil
	})
	h.require.NoErrorf(err, "should have deleted game data directories")
}

func (h *Helper) gameDataDir(addr common.Address) string {
	return filepath.Join(h.dir, "game-"+addr.Hex())
}
