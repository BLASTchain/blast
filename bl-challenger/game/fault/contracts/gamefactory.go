package contracts

import (
	"context"
	"fmt"
	"math/big"

	"github.com/BLASTchain/blast/bl-bindings/bindings"
	"github.com/BLASTchain/blast/bl-challenger/game/types"
	"github.com/BLASTchain/blast/bl-service/sources/batching"
	"github.com/ethereum/go-ethereum/common"
)

const (
	methodGameCount   = "gameCount"
	methodGameAtIndex = "gameAtIndex"
)

type DisputeGameFactoryContract struct {
	multiCaller *batching.MultiCaller
	contract    *batching.BoundContract
}

func NewDisputeGameFactoryContract(addr common.Address, caller *batching.MultiCaller) (*DisputeGameFactoryContract, error) {
	factoryAbi, err := bindings.DisputeGameFactoryMetaData.GetAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to load dispute game factory ABI: %w", err)
	}
	return &DisputeGameFactoryContract{
		multiCaller: caller,
		contract:    batching.NewBoundContract(factoryAbi, addr),
	}, nil
}

func (f *DisputeGameFactoryContract) GetGameCount(ctx context.Context, blockNum uint64) (uint64, error) {
	result, err := f.multiCaller.SingleCall(ctx, batching.BlockByNumber(blockNum), f.contract.Call(methodGameCount))
	if err != nil {
		return 0, fmt.Errorf("failed to load game count: %w", err)
	}
	return result.GetBigInt(0).Uint64(), nil
}

func (f *DisputeGameFactoryContract) GetGame(ctx context.Context, idx uint64, blockNum uint64) (types.GameMetadata, error) {
	result, err := f.multiCaller.SingleCall(ctx, batching.BlockByNumber(blockNum), f.contract.Call(methodGameAtIndex, new(big.Int).SetUint64(idx)))
	if err != nil {
		return types.GameMetadata{}, fmt.Errorf("failed to load game %v: %w", idx, err)
	}
	return f.decodeGame(result), nil
}

func (f *DisputeGameFactoryContract) decodeGame(result *batching.CallResult) types.GameMetadata {
	gameType := result.GetUint8(0)
	timestamp := result.GetUint64(1)
	proxy := result.GetAddress(2)
	return types.GameMetadata{
		GameType:  gameType,
		Timestamp: timestamp,
		Proxy:     proxy,
	}
}
