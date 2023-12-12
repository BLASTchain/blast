package contracts

import (
	"math/big"

	"github.com/BLASTchain/blast/indexer/bigint"
	"github.com/BLASTchain/blast/indexer/database"
	legacy_bindings "github.com/BLASTchain/blast/bl-bindings/legacy-bindings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type LegacyCTCDepositEvent struct {
	Event    *database.ContractEvent
	Tx       database.Transaction
	TxHash   common.Hash
	GasLimit *big.Int
}

func LegacyCTCDepositEvents(contractAddress common.Address, db *database.DB, fromHeight, toHeight *big.Int) ([]LegacyCTCDepositEvent, error) {
	ctcAbi, err := legacy_bindings.CanonicalTransactionChainMetaData.GetAbi()
	if err != nil {
		return nil, err
	}

	transactionEnqueuedEventAbi := ctcAbi.Events["TransactionEnqueued"]
	contractEventFilter := database.ContractEvent{ContractAddress: contractAddress, EventSignature: transactionEnqueuedEventAbi.ID}
	events, err := db.ContractEvents.L1ContractEventsWithFilter(contractEventFilter, fromHeight, toHeight)
	if err != nil {
		return nil, err
	}

	ctcTxDeposits := make([]LegacyCTCDepositEvent, len(events))
	for i := range events {
		txEnqueued := legacy_bindings.CanonicalTransactionChainTransactionEnqueued{Raw: *events[i].RLPLog}
		err = UnpackLog(&txEnqueued, events[i].RLPLog, transactionEnqueuedEventAbi.Name, ctcAbi)
		if err != nil {
			return nil, err
		}

		// Enqueued Deposits do not carry a `msg.value` amount. ETH is only minted on L2 via the L1StandardBrige
		ctcTxDeposits[i] = LegacyCTCDepositEvent{
			Event:    &events[i].ContractEvent,
			GasLimit: txEnqueued.GasLimit,
			TxHash:   types.NewTransaction(0, txEnqueued.Target, bigint.Zero, txEnqueued.GasLimit.Uint64(), nil, txEnqueued.Data).Hash(),
			Tx: database.Transaction{
				FromAddress: txEnqueued.L1TxOrigin,
				ToAddress:   txEnqueued.Target,
				Amount:      bigint.Zero,
				Data:        txEnqueued.Data,
				Timestamp:   events[i].Timestamp,
			},
		}
	}

	return ctcTxDeposits, nil
}
