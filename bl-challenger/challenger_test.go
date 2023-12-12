package op_challenger

import (
	"context"
	"testing"

	"github.com/BLASTchain/blast/bl-challenger/config"
	"github.com/BLASTchain/blast/bl-service/testlog"
	"github.com/ethereum/go-ethereum/log"
	"github.com/stretchr/testify/require"
)

func TestMainShouldReturnErrorWhenConfigInvalid(t *testing.T) {
	cfg := &config.Config{}
	err := Main(context.Background(), testlog.Logger(t, log.LvlInfo), cfg)
	require.ErrorIs(t, err, cfg.Check())
}
