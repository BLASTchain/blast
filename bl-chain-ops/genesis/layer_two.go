package genesis

import (
	"fmt"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"

	"github.com/BLASTchain/blast/bl-bindings/predeploys"
	"github.com/BLASTchain/blast/bl-chain-ops/immutables"
	"github.com/BLASTchain/blast/bl-chain-ops/state"
	"github.com/BLASTchain/blast/bl-service/eth"
)

// BuildL2DeveloperGenesis will build the L2 genesis block.
func BuildL2Genesis(config *DeployConfig, l1StartBlock *types.Block) (*core.Genesis, error) {
	genspec, err := NewL2Genesis(config, l1StartBlock)
	if err != nil {
		return nil, err
	}

	db := state.NewMemoryStateDB(genspec)
	if config.FundDevAccounts {
		log.Info("Funding developer accounts in L2 genesis")
		FundDevAccounts(db)
	}

	SetPrecompileBalances(db)

	storage, err := NewL2StorageConfig(config, l1StartBlock)
	if err != nil {
		return nil, err
	}

	immutable, err := NewL2ImmutableConfig(config, l1StartBlock)
	if err != nil {
		return nil, err
	}

	// Set up the proxies
	err = setProxies(db, predeploys.ProxyAdminAddr, BigL2PredeployNamespace, 2048)
	if err != nil {
		return nil, err
	}

	// Set up the implementations
	deployResults, err := immutables.BuildOptimism(immutable)
	if err != nil {
		return nil, err
	}
	for name, predeploy := range predeploys.Predeploys {
		addr := *predeploy
		if addr == predeploys.GovernanceTokenAddr && !config.EnableGovernance {
			// there is no governance token configured, so skip the governance token predeploy
			log.Warn("Governance is not enabled, skipping governance token predeploy.")
			continue
		}
		codeAddr := addr
		if predeploys.IsProxied(addr) {
			codeAddr, err = AddressToCodeNamespace(addr)
			if err != nil {
				return nil, fmt.Errorf("error converting to code namespace: %w", err)
			}
			db.CreateAccount(codeAddr)
			db.SetState(addr, ImplementationSlot, eth.AddressAsLeftPaddedHash(codeAddr))
			log.Info("Set proxy", "name", name, "address", addr, "implementation", codeAddr)
		} else {
			db.DeleteState(addr, AdminSlot)
		}
		if err := setupPredeploy(db, deployResults, storage, name, addr, codeAddr); err != nil {
			return nil, err
		}
		code := db.GetCode(codeAddr)
		if len(code) == 0 {
			return nil, fmt.Errorf("code not set for %s", name)
		}
	}

	return db.Genesis(), nil
}
