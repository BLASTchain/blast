package scheduler

import (
	"context"

	"github.com/ethereum/go-ethereum/common"

	"github.com/BLASTchain/blast/bl-challenger/game/types"
)

type GamePlayer interface {
	ProgressGame(ctx context.Context) types.GameStatus
	Status() types.GameStatus
}

type DiskManager interface {
	DirForGame(addr common.Address) string
	RemoveAllExcept(addrs []common.Address) error
}

type job struct {
	addr   common.Address
	player GamePlayer
	status types.GameStatus
}
