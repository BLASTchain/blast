package clients

import (
	"context"
	"math/big"
	"time"

	"github.com/BLASTchain/blast/op-ufm/pkg/metrics"

	optls "github.com/BLASTchain/blast/bl-service/tls"
	signer "github.com/BLASTchain/blast/bl-service/signer"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

type InstrumentedSignerClient struct {
	c            *signer.SignerClient
	providerName string
}

func NewSignerClient(providerName string, logger log.Logger, endpoint string, tlsConfig optls.CLIConfig) (*InstrumentedSignerClient, error) {
	start := time.Now()
	c, err := signer.NewSignerClient(logger, endpoint, tlsConfig)
	if err != nil {
		metrics.RecordErrorDetails(providerName, "signer.NewSignerClient", err)
		return nil, err
	}
	metrics.RecordRPCLatency(providerName, "signer", "NewSignerClient", time.Since(start))
	return &InstrumentedSignerClient{c: c, providerName: providerName}, nil
}

func (i *InstrumentedSignerClient) SignTransaction(ctx context.Context, chainId *big.Int, tx *types.Transaction) (*types.Transaction, error) {
	start := time.Now()
	tx, err := i.c.SignTransaction(ctx, chainId, tx)
	if err != nil {
		metrics.RecordErrorDetails(i.providerName, "signer.SignTransaction", err)
		return nil, err
	}
	metrics.RecordRPCLatency(i.providerName, "signer", "SignTransaction", time.Since(start))
	return tx, err
}
