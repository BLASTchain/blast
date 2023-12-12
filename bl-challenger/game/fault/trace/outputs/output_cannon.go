package outputs

import (
	"context"
	"errors"

	"github.com/BLASTchain/blast/bl-challenger/game/fault/trace"
	"github.com/BLASTchain/blast/bl-challenger/game/fault/types"
	"github.com/ethereum/go-ethereum/log"
)

func NewOutputCannonTraceAccessor(ctx context.Context, logger log.Logger, rollupRpc string, gameDepth uint64, prestateBlock uint64, poststateBlock uint64) (*trace.Accessor, error) {
	topDepth := gameDepth / 2 // TODO(client-pod#43): Load this from the contract
	outputProvider, err := NewTraceProvider(ctx, logger, rollupRpc, topDepth, prestateBlock, poststateBlock)
	if err != nil {
		return nil, err
	}

	cannonCreator := func(ctx context.Context, pre types.Claim, post types.Claim) (types.TraceProvider, error) {
		// TODO(client-pod#43): Actually create the cannon trace provider for the trace between the given claims.
		return nil, errors.New("not implemented")
	}

	selector := newSplitProviderSelector(outputProvider, int(topDepth), cannonCreator)
	return trace.NewAccessor(selector), nil
}
