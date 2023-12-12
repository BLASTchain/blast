package op_challenger

import (
	"context"
	"fmt"

	"github.com/BLASTchain/blast/bl-challenger/config"
	"github.com/BLASTchain/blast/bl-challenger/game"
	"github.com/ethereum/go-ethereum/log"
)

// Main is the programmatic entry-point for running bl-challenger
func Main(ctx context.Context, logger log.Logger, cfg *config.Config) error {
	if err := cfg.Check(); err != nil {
		return err
	}
	service, err := game.NewService(ctx, logger, cfg)
	if err != nil {
		return fmt.Errorf("failed to create the fault service: %w", err)
	}

	return service.MonitorGame(ctx)
}
