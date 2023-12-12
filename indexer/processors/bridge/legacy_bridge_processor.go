package bridge

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"

	"github.com/BLASTchain/blast/indexer/bigint"
	"github.com/BLASTchain/blast/indexer/config"
	"github.com/BLASTchain/blast/indexer/database"
	"github.com/BLASTchain/blast/indexer/node"
	"github.com/BLASTchain/blast/indexer/processors/contracts"
)

// Legacy Bridge Initiation

// LegacyL1ProcessInitiatedEvents will query the data for bridge events within the specified block range
// according the pre-bedrock protocol. This follows:
//  1. CanonicalTransactionChain
//  2. L1CrossDomainMessenger
//  3. L1StandardBridge
func LegacyL1ProcessInitiatedBridgeEvents(log log.Logger, db *database.DB, metrics L1Metricer, l1Contracts config.L1Contracts, fromHeight, toHeight *big.Int) error {
	// (1) CanonicalTransactionChain
	ctcTxDepositEvents, err := contracts.LegacyCTCDepositEvents(l1Contracts.LegacyCanonicalTransactionChain, db, fromHeight, toHeight)
	if err != nil {
		return err
	}
	if len(ctcTxDepositEvents) > 0 {
		log.Info("detected legacy transaction deposits", "size", len(ctcTxDepositEvents))
	}

	mintedWEI := bigint.Zero
	ctcTxDeposits := make(map[logKey]*contracts.LegacyCTCDepositEvent, len(ctcTxDepositEvents))
	transactionDeposits := make([]database.L1TransactionDeposit, len(ctcTxDepositEvents))
	for i := range ctcTxDepositEvents {
		deposit := ctcTxDepositEvents[i]
		ctcTxDeposits[logKey{deposit.Event.BlockHash, deposit.Event.LogIndex}] = &deposit
		mintedWEI = new(big.Int).Add(mintedWEI, deposit.Tx.Amount)

		// We re-use the L2 Transaction hash as the source hash to remain consistent in the schema.
		transactionDeposits[i] = database.L1TransactionDeposit{
			SourceHash:           deposit.TxHash,
			L2TransactionHash:    deposit.TxHash,
			InitiatedL1EventGUID: deposit.Event.GUID,
			GasLimit:             deposit.GasLimit,
			Tx:                   deposit.Tx,
		}
	}
	if len(ctcTxDepositEvents) > 0 {
		if err := db.BridgeTransactions.StoreL1TransactionDeposits(transactionDeposits); err != nil {
			return err
		}

		mintedETH, _ := bigint.WeiToETH(mintedWEI).Float64()
		metrics.RecordL1TransactionDeposits(len(transactionDeposits), mintedETH)
	}

	// (2) L1CrossDomainMessenger
	crossDomainSentMessages, err := contracts.CrossDomainMessengerSentMessageEvents("l1", l1Contracts.L1CrossDomainMessengerProxy, db, fromHeight, toHeight)
	if err != nil {
		return err
	}
	if len(crossDomainSentMessages) > 0 {
		log.Info("detected legacy sent messages", "size", len(crossDomainSentMessages))
	}

	sentMessages := make(map[logKey]*contracts.CrossDomainMessengerSentMessageEvent, len(crossDomainSentMessages))
	bridgeMessages := make([]database.L1BridgeMessage, len(crossDomainSentMessages))
	for i := range crossDomainSentMessages {
		sentMessage := crossDomainSentMessages[i]
		sentMessages[logKey{sentMessage.Event.BlockHash, sentMessage.Event.LogIndex}] = &sentMessage

		// extract the deposit hash from the previous TransactionDepositedEvent
		ctcTxDeposit, ok := ctcTxDeposits[logKey{sentMessage.Event.BlockHash, sentMessage.Event.LogIndex - 1}]
		if !ok {
			return fmt.Errorf("expected TransactionEnqueued preceding SentMessage event. tx_hash = %s", sentMessage.Event.TransactionHash)
		} else if ctcTxDeposit.Event.TransactionHash != sentMessage.Event.TransactionHash {
			return fmt.Errorf("correlated events tx hash mismatch. deposit_tx_hash = %s, message_tx_hash = %s", ctcTxDeposit.Event.TransactionHash, sentMessage.Event.TransactionHash)
		}

		bridgeMessages[i] = database.L1BridgeMessage{TransactionSourceHash: ctcTxDeposit.TxHash, BridgeMessage: sentMessage.BridgeMessage}
	}
	if len(bridgeMessages) > 0 {
		if err := db.BridgeMessages.StoreL1BridgeMessages(bridgeMessages); err != nil {
			return err
		}
		metrics.RecordL1CrossDomainSentMessages(len(bridgeMessages))
	}

	// (3) L1StandardBridge
	initiatedBridges, err := contracts.L1StandardBridgeLegacyDepositInitiatedEvents(l1Contracts.L1StandardBridgeProxy, db, fromHeight, toHeight)
	if err != nil {
		return err
	}
	if len(initiatedBridges) > 0 {
		log.Info("detected iegacy bridge deposits", "size", len(initiatedBridges))
	}

	bridgedTokens := make(map[common.Address]int)
	bridgeDeposits := make([]database.L1BridgeDeposit, len(initiatedBridges))
	for i := range initiatedBridges {
		initiatedBridge := initiatedBridges[i]

		// extract the cross domain message hash & deposit source hash from the following events
		// Unlike bedrock, the bridge events are emitted AFTER sending the cross domain message
		// 	- Event Flow: TransactionEnqueued -> SentMessage -> DepositInitiated
		sentMessage, ok := sentMessages[logKey{initiatedBridge.Event.BlockHash, initiatedBridge.Event.LogIndex - 1}]
		if !ok {
			return fmt.Errorf("expected SentMessage preceding DepositInitiated event. tx_hash = %s", initiatedBridge.Event.TransactionHash)
		} else if sentMessage.Event.TransactionHash != initiatedBridge.Event.TransactionHash {
			return fmt.Errorf("correlated events tx hash mismatch. bridge_tx_hash = %s, message_tx_hash = %s", initiatedBridge.Event.TransactionHash, sentMessage.Event.TransactionHash)
		}

		ctcTxDeposit, ok := ctcTxDeposits[logKey{initiatedBridge.Event.BlockHash, initiatedBridge.Event.LogIndex - 2}]
		if !ok {
			return fmt.Errorf("expected TransactionEnqueued preceding BridgeInitiated event. tx_hash = %s", initiatedBridge.Event.TransactionHash)
		} else if ctcTxDeposit.Event.TransactionHash != initiatedBridge.Event.TransactionHash {
			return fmt.Errorf("correlated events tx hash mismatch. bridge_tx_hash = %s, deposit_tx_hash = %s", initiatedBridge.Event.TransactionHash, ctcTxDeposit.Event.TransactionHash)
		}

		initiatedBridge.BridgeTransfer.CrossDomainMessageHash = &sentMessage.BridgeMessage.MessageHash
		bridgedTokens[initiatedBridge.BridgeTransfer.TokenPair.LocalTokenAddress]++
		bridgeDeposits[i] = database.L1BridgeDeposit{
			TransactionSourceHash: ctcTxDeposit.TxHash,
			BridgeTransfer:        initiatedBridge.BridgeTransfer,
		}
	}
	if len(bridgeDeposits) > 0 {
		if err := db.BridgeTransfers.StoreL1BridgeDeposits(bridgeDeposits); err != nil {
			return err
		}
		for tokenAddr, size := range bridgedTokens {
			metrics.RecordL1InitiatedBridgeTransfers(tokenAddr, size)
		}
	}

	// a-ok!
	return nil
}

// LegacyL2ProcessInitiatedEvents will query the data for bridge events within the specified block range
// according the pre-bedrock protocol. This follows:
//  1. L2CrossDomainMessenger - The LegacyMessagePasser contract cannot be used as entrypoint to bridge transactions from L2. The protocol
//     only allows the L2CrossDomainMessenger as the sole sender when relaying a bridged message.
//  2. L2StandardBridge
func LegacyL2ProcessInitiatedBridgeEvents(log log.Logger, db *database.DB, metrics L2Metricer, l2Contracts config.L2Contracts, fromHeight, toHeight *big.Int) error {
	// (1) L2CrossDomainMessenger
	crossDomainSentMessages, err := contracts.CrossDomainMessengerSentMessageEvents("l2", l2Contracts.L2CrossDomainMessenger, db, fromHeight, toHeight)
	if err != nil {
		return err
	}
	if len(crossDomainSentMessages) > 0 {
		log.Info("detected legacy transaction withdrawals (via L2CrossDomainMessenger)", "size", len(crossDomainSentMessages))
	}

	type sentMessageEvent struct {
		*contracts.CrossDomainMessengerSentMessageEvent
		WithdrawalHash common.Hash
	}

	withdrawnWEI := bigint.Zero
	sentMessages := make(map[logKey]sentMessageEvent, len(crossDomainSentMessages))
	bridgeMessages := make([]database.L2BridgeMessage, len(crossDomainSentMessages))
	transactionWithdrawals := make([]database.L2TransactionWithdrawal, len(crossDomainSentMessages))
	for i := range crossDomainSentMessages {
		sentMessage := crossDomainSentMessages[i]
		withdrawnWEI = new(big.Int).Add(withdrawnWEI, sentMessage.BridgeMessage.Tx.Amount)

		// We re-use the L2CrossDomainMessenger message hash as the withdrawal hash to remain consistent in the schema.
		transactionWithdrawals[i] = database.L2TransactionWithdrawal{
			WithdrawalHash:       sentMessage.BridgeMessage.MessageHash,
			InitiatedL2EventGUID: sentMessage.Event.GUID,
			Nonce:                sentMessage.BridgeMessage.Nonce,
			GasLimit:             sentMessage.BridgeMessage.GasLimit,
			Tx: database.Transaction{
				FromAddress: sentMessage.BridgeMessage.Tx.FromAddress,
				ToAddress:   sentMessage.BridgeMessage.Tx.ToAddress,
				Amount:      big.NewInt(0),
				Data:        sentMessage.BridgeMessage.Tx.Data,
				Timestamp:   sentMessage.Event.Timestamp,
			},
		}

		sentMessages[logKey{sentMessage.Event.BlockHash, sentMessage.Event.LogIndex}] = sentMessageEvent{&sentMessage, sentMessage.BridgeMessage.MessageHash}
		bridgeMessages[i] = database.L2BridgeMessage{
			TransactionWithdrawalHash: sentMessage.BridgeMessage.MessageHash,
			BridgeMessage:             sentMessage.BridgeMessage,
		}
	}
	if len(bridgeMessages) > 0 {
		if err := db.BridgeTransactions.StoreL2TransactionWithdrawals(transactionWithdrawals); err != nil {
			return err
		}
		if err := db.BridgeMessages.StoreL2BridgeMessages(bridgeMessages); err != nil {
			return err
		}

		withdrawnETH, _ := bigint.WeiToETH(withdrawnWEI).Float64()
		metrics.RecordL2TransactionWithdrawals(len(transactionWithdrawals), withdrawnETH)
		metrics.RecordL2CrossDomainSentMessages(len(bridgeMessages))
	}

	// (2) L2StandardBridge
	initiatedBridges, err := contracts.L2StandardBridgeLegacyWithdrawalInitiatedEvents(l2Contracts.L2StandardBridge, db, fromHeight, toHeight)
	if err != nil {
		return err
	}
	if len(initiatedBridges) > 0 {
		log.Info("detected legacy bridge withdrawals", "size", len(initiatedBridges))
	}

	bridgedTokens := make(map[common.Address]int)
	l2BridgeWithdrawals := make([]database.L2BridgeWithdrawal, len(initiatedBridges))
	for i := range initiatedBridges {
		initiatedBridge := initiatedBridges[i]

		// extract the cross domain message hash & deposit source hash from the following events
		// Unlike bedrock, the bridge events are emitted AFTER sending the cross domain message
		// 	- Event Flow: TransactionEnqueued -> SentMessage -> DepositInitiated
		sentMessage, ok := sentMessages[logKey{initiatedBridge.Event.BlockHash, initiatedBridge.Event.LogIndex - 1}]
		if !ok {
			return fmt.Errorf("expected SentMessage preceding BridgeInitiated event. tx_hash = %s", initiatedBridge.Event.TransactionHash)
		} else if sentMessage.Event.TransactionHash != initiatedBridge.Event.TransactionHash {
			return fmt.Errorf("correlated events tx hash mismatch. bridge_tx_hash = %s, message_tx_hash = %s", initiatedBridge.Event.TransactionHash, sentMessage.Event.TransactionHash)
		}

		bridgedTokens[initiatedBridge.BridgeTransfer.TokenPair.LocalTokenAddress]++
		initiatedBridge.BridgeTransfer.CrossDomainMessageHash = &sentMessage.BridgeMessage.MessageHash
		l2BridgeWithdrawals[i] = database.L2BridgeWithdrawal{
			TransactionWithdrawalHash: sentMessage.WithdrawalHash,
			BridgeTransfer:            initiatedBridge.BridgeTransfer,
		}
	}
	if len(l2BridgeWithdrawals) > 0 {
		if err := db.BridgeTransfers.StoreL2BridgeWithdrawals(l2BridgeWithdrawals); err != nil {
			return err
		}
		for tokenAddr, size := range bridgedTokens {
			metrics.RecordL2InitiatedBridgeTransfers(tokenAddr, size)
		}
	}

	// a-ok
	return nil
}

// Legacy Bridge Finalization

// LegacyL1ProcessFinalizedBridgeEvents will query for bridge events within the specified block range
// according to the pre-bedrock protocol. This follows:
//  1. L1CrossDomainMessenger
//  2. L1StandardBridge
func LegacyL1ProcessFinalizedBridgeEvents(log log.Logger, db *database.DB, metrics L1Metricer, l1Client node.EthClient, l1Contracts config.L1Contracts, fromHeight, toHeight *big.Int) error {
	// (1) L1CrossDomainMessenger -> This is the root-most contract from which bridge events are finalized since withdrawals must be initiated from the
	// L2CrossDomainMessenger. Since there's no two-step withdrawal process, we mark the transaction as proven/finalized in the same step
	crossDomainRelayedMessages, err := contracts.CrossDomainMessengerRelayedMessageEvents("l1", l1Contracts.L1CrossDomainMessengerProxy, db, fromHeight, toHeight)
	if err != nil {
		return err
	}
	if len(crossDomainRelayedMessages) > 0 {
		log.Info("detected relayed messages", "size", len(crossDomainRelayedMessages))
	}

	skippedPreRegenesisMessages := 0
	for i := range crossDomainRelayedMessages {
		relayedMessage := crossDomainRelayedMessages[i]
		message, err := db.BridgeMessages.L2BridgeMessage(relayedMessage.MessageHash)
		if err != nil {
			return err
		} else if message == nil {
			// Before surfacing an error about a missing withdrawal, we need to handle an edge case
			// for OP-Mainnet pre-regensis withdrawals that no longer exist on L2.
			tx, err := l1Client.TxByHash(relayedMessage.Event.TransactionHash)
			if err != nil {
				return fmt.Errorf("unable to query legacy relayed. tx_hash = %s: %w", relayedMessage.Event.TransactionHash, err)
			} else if tx == nil {
				return fmt.Errorf("missing tx for relayed message! tx_hash = %s", relayedMessage.Event.TransactionHash)
			}

			relayMessageData := tx.Data()[4:]
			inputs, err := contracts.CrossDomainMessengerLegacyRelayMessageEncoding.Inputs.Unpack(relayMessageData)
			if err != nil || inputs == nil {
				return fmt.Errorf("unable to extract XDomainCallData from relayMessage transaction. tx_hash = %s: %w", relayedMessage.Event.TransactionHash, err)
			}

			// NOTE: Since OP-Mainnet is the only network to go through a regensis we can simply harcode the
			// the starting message nonce at genesis (100k). Any relayed withdrawal on L1 with a lesser nonce
			// is a clear indicator of a pre-regenesis withdrawal.
			if inputs[3].(*big.Int).Int64() < 100_000 {
				// skip pre-regenesis withdrawals
				skippedPreRegenesisMessages++
				continue
			} else {
				return fmt.Errorf("missing indexed L2CrossDomainMessenger message! tx_hash = %s", relayedMessage.Event.TransactionHash)
			}
		}

		// Mark the associated tx withdrawal as proven/finalized with the same event. The message hash is also the transaction withdrawal hash
		if err := db.BridgeTransactions.MarkL2TransactionWithdrawalProvenEvent(relayedMessage.MessageHash, relayedMessage.Event.GUID); err != nil {
			return fmt.Errorf("failed to mark withdrawal as proven. tx_hash = %s: %w", relayedMessage.Event.TransactionHash, err)
		}
		if err := db.BridgeTransactions.MarkL2TransactionWithdrawalFinalizedEvent(relayedMessage.MessageHash, relayedMessage.Event.GUID, true); err != nil {
			return fmt.Errorf("failed to mark withdrawal as finalized. tx_hash = %s: %w", relayedMessage.Event.TransactionHash, err)
		}
		if err := db.BridgeMessages.MarkRelayedL2BridgeMessage(relayedMessage.MessageHash, relayedMessage.Event.GUID); err != nil {
			return fmt.Errorf("failed to relay cross domain message. tx_hash = %s: %w", relayedMessage.Event.TransactionHash, err)
		}
	}
	if len(crossDomainRelayedMessages) > 0 {
		metrics.RecordL1ProvenWithdrawals(len(crossDomainRelayedMessages))
		metrics.RecordL1FinalizedWithdrawals(len(crossDomainRelayedMessages))
		metrics.RecordL1CrossDomainRelayedMessages(len(crossDomainRelayedMessages))
	}
	if skippedPreRegenesisMessages > 0 {
		// Logged as a warning just for visibility
		log.Warn("skipped pre-regensis relayed L2CrossDomainMessenger withdrawals", "size", skippedPreRegenesisMessages)
	}

	// (2) L1StandardBridge
	// 	- Nothing actionable on the database. Since the StandardBridge is layered ontop of the
	// CrossDomainMessenger, there's no need for any sanity or invariant checks as the previous step
	// ensures a relayed message (finalized bridge) can be linked with a sent message (initiated bridge).

	//  - NOTE: Ignoring metrics for pre-bedrock transfers

	// a-ok!
	return nil
}

// LegacyL2ProcessFinalizedBridgeEvents will query for bridge events within the specified block range
// according to the pre-bedrock protocol. This follows:
//  1. L2CrossDomainMessenger
//  2. L2StandardBridge
func LegacyL2ProcessFinalizedBridgeEvents(log log.Logger, db *database.DB, metrics L2Metricer, l2Contracts config.L2Contracts, fromHeight, toHeight *big.Int) error {
	// (1) L2CrossDomainMessenger
	crossDomainRelayedMessages, err := contracts.CrossDomainMessengerRelayedMessageEvents("l2", l2Contracts.L2CrossDomainMessenger, db, fromHeight, toHeight)
	if err != nil {
		return err
	}
	if len(crossDomainRelayedMessages) > 0 {
		log.Info("detected relayed legacy messages", "size", len(crossDomainRelayedMessages))
	}

	for i := range crossDomainRelayedMessages {
		relayedMessage := crossDomainRelayedMessages[i]
		message, err := db.BridgeMessages.L1BridgeMessage(relayedMessage.MessageHash)
		if err != nil {
			return err
		} else if message == nil {
			return fmt.Errorf("missing indexed L1CrossDomainMessager message! tx_hash = %s", relayedMessage.Event.TransactionHash)
		}

		if err := db.BridgeMessages.MarkRelayedL1BridgeMessage(relayedMessage.MessageHash, relayedMessage.Event.GUID); err != nil {
			return fmt.Errorf("failed to relay cross domain message: %w", err)
		}
	}
	if len(crossDomainRelayedMessages) > 0 {
		metrics.RecordL2CrossDomainRelayedMessages(len(crossDomainRelayedMessages))
	}

	// (2) L2StandardBridge
	// 	- Nothing actionable on the database. Since the StandardBridge is layered ontop of the
	// CrossDomainMessenger, there's no need for any sanity or invariant checks as the previous step
	// ensures a relayed message (finalized bridge) can be linked with a sent message (initiated bridge).

	//  - NOTE: Ignoring metrics for pre-bedrock transfers

	// a-ok!
	return nil
}
