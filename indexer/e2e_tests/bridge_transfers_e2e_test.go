package e2e_tests

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/BLASTchain/blast/indexer/bigint"
	e2etest_utils "github.com/BLASTchain/blast/indexer/e2e_tests/utils"
	op_e2e "github.com/BLASTchain/blast/bl-e2e"
	"github.com/BLASTchain/blast/bl-e2e/e2eutils/transactions"
	"github.com/BLASTchain/blast/bl-e2e/e2eutils/wait"
	"github.com/BLASTchain/blast/bl-node/withdrawals"

	"github.com/BLASTchain/blast/bl-bindings/bindings"
	"github.com/BLASTchain/blast/bl-bindings/predeploys"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"

	"github.com/stretchr/testify/require"
)

func TestE2EBridgeTransfersStandardBridgeETHDeposit(t *testing.T) {
	testSuite := createE2ETestSuite(t)

	l1StandardBridge, err := bindings.NewL1StandardBridge(testSuite.OpCfg.L1Deployments.L1StandardBridgeProxy, testSuite.L1Client)
	require.NoError(t, err)

	// 1 ETH transfer
	aliceAddr := testSuite.OpCfg.Secrets.Addresses().Alice
	l1Opts, err := bind.NewKeyedTransactorWithChainID(testSuite.OpCfg.Secrets.Alice, testSuite.OpCfg.L1ChainIDBig())
	require.NoError(t, err)
	l1Opts.Value = big.NewInt(params.Ether)

	// (1) Test Deposit Initiation
	depositTx, err := l1StandardBridge.DepositETH(l1Opts, 200_000, []byte{byte(1)})
	require.NoError(t, err)
	depositReceipt, err := wait.ForReceiptOK(context.Background(), testSuite.L1Client, depositTx.Hash())
	require.NoError(t, err)

	depositInfo, err := e2etest_utils.ParseDepositInfo(depositReceipt)
	require.NoError(t, err)

	// wait for processor catchup
	require.NoError(t, wait.For(context.Background(), 500*time.Millisecond, func() (bool, error) {
		l1Header := testSuite.Indexer.BridgeProcessor.LastL1Header
		return l1Header != nil && l1Header.Number.Uint64() >= depositReceipt.BlockNumber.Uint64(), nil
	}))

	cursor := ""
	limit := 100

	aliceDeposits, err := testSuite.DB.BridgeTransfers.L1BridgeDepositsByAddress(aliceAddr, cursor, limit)

	require.NoError(t, err)
	require.Len(t, aliceDeposits.Deposits, 1)
	require.Equal(t, depositTx.Hash(), aliceDeposits.Deposits[0].L1TransactionHash)
	require.Equal(t, depositReceipt.BlockHash, aliceDeposits.Deposits[0].L1BlockHash)
	require.Equal(t, "", aliceDeposits.Cursor)
	require.Equal(t, false, aliceDeposits.HasNextPage)
	require.Equal(t, types.NewTx(depositInfo.DepositTx).Hash().String(), aliceDeposits.Deposits[0].L2TransactionHash.String())

	deposit := aliceDeposits.Deposits[0].L1BridgeDeposit
	require.Equal(t, depositInfo.DepositTx.SourceHash, deposit.TransactionSourceHash)
	require.Equal(t, predeploys.LegacyERC20ETHAddr, deposit.TokenPair.LocalTokenAddress)
	require.Equal(t, predeploys.LegacyERC20ETHAddr, deposit.TokenPair.RemoteTokenAddress)
	require.Equal(t, uint64(params.Ether), deposit.Tx.Amount.Uint64())
	require.Equal(t, aliceAddr, deposit.Tx.FromAddress)
	require.Equal(t, aliceAddr, deposit.Tx.ToAddress)
	require.Equal(t, byte(1), deposit.Tx.Data[0])

	// StandardBridge flows through the messenger. We remove the first two significant
	// bytes of the nonce dedicated to the version. nonce == 0 (first message)
	require.NotNil(t, deposit.CrossDomainMessageHash)

	// (2) Test Deposit Finalization via CrossDomainMessenger relayed message
	l2DepositReceipt, err := wait.ForReceiptOK(context.Background(), testSuite.L2Client, types.NewTx(depositInfo.DepositTx).Hash())
	require.NoError(t, err)
	require.NoError(t, wait.For(context.Background(), 500*time.Millisecond, func() (bool, error) {
		l2Header := testSuite.Indexer.BridgeProcessor.LastFinalizedL2Header
		return l2Header != nil && l2Header.Number.Uint64() >= l2DepositReceipt.BlockNumber.Uint64(), nil
	}))

	crossDomainBridgeMessage, err := testSuite.DB.BridgeMessages.L1BridgeMessage(*deposit.CrossDomainMessageHash)
	require.NoError(t, err)
	require.NotNil(t, crossDomainBridgeMessage)
	require.NotNil(t, crossDomainBridgeMessage.RelayedMessageEventGUID)
}

func TestE2EBridgeTransfersOptimismPortalETHReceive(t *testing.T) {
	testSuite := createE2ETestSuite(t)

	optimismPortal, err := bindings.NewOptimismPortal(testSuite.OpCfg.L1Deployments.OptimismPortalProxy, testSuite.L1Client)
	require.NoError(t, err)

	// 1 ETH transfer
	aliceAddr := testSuite.OpCfg.Secrets.Addresses().Alice
	l1Opts, err := bind.NewKeyedTransactorWithChainID(testSuite.OpCfg.Secrets.Alice, testSuite.OpCfg.L1ChainIDBig())
	require.NoError(t, err)
	l1Opts.Value = big.NewInt(params.Ether)

	// (1) Test Deposit Initiation
	portalDepositTx, err := optimismPortal.Receive(l1Opts)
	require.NoError(t, err)
	portalDepositReceipt, err := wait.ForReceiptOK(context.Background(), testSuite.L1Client, portalDepositTx.Hash())
	require.NoError(t, err)

	depositInfo, err := e2etest_utils.ParseDepositInfo(portalDepositReceipt)
	require.NoError(t, err)

	// wait for processor catchup
	require.NoError(t, wait.For(context.Background(), 500*time.Millisecond, func() (bool, error) {
		l1Header := testSuite.Indexer.BridgeProcessor.LastL1Header
		return l1Header != nil && l1Header.Number.Uint64() >= portalDepositReceipt.BlockNumber.Uint64(), nil
	}))

	aliceDeposits, err := testSuite.DB.BridgeTransfers.L1BridgeDepositsByAddress(aliceAddr, "", 1)
	require.NoError(t, err)
	require.NotNil(t, aliceDeposits)
	require.Len(t, aliceDeposits.Deposits, 1)
	require.Equal(t, portalDepositTx.Hash(), aliceDeposits.Deposits[0].L1TransactionHash)

	deposit := aliceDeposits.Deposits[0].L1BridgeDeposit
	require.Equal(t, depositInfo.DepositTx.SourceHash, deposit.TransactionSourceHash)
	require.Equal(t, predeploys.LegacyERC20ETHAddr, deposit.TokenPair.LocalTokenAddress)
	require.Equal(t, predeploys.LegacyERC20ETHAddr, deposit.TokenPair.RemoteTokenAddress)
	require.Equal(t, uint64(params.Ether), deposit.Tx.Amount.Uint64())
	require.Equal(t, aliceAddr, deposit.Tx.FromAddress)
	require.Equal(t, aliceAddr, deposit.Tx.ToAddress)
	require.Len(t, deposit.Tx.Data, 0)

	// deposit was not sent through the cross domain messenger
	require.Nil(t, deposit.CrossDomainMessageHash)

	// (2) Test Deposit Finalization
	l2DepositReceipt, err := wait.ForReceiptOK(context.Background(), testSuite.L2Client, types.NewTx(depositInfo.DepositTx).Hash())
	require.NoError(t, err)
	require.NoError(t, wait.For(context.Background(), 500*time.Millisecond, func() (bool, error) {
		l2Header := testSuite.Indexer.BridgeProcessor.LastFinalizedL2Header
		return l2Header != nil && l2Header.Number.Uint64() >= l2DepositReceipt.BlockNumber.Uint64(), nil
	}))

	// Still nil as the withdrawal did not occur through the standard bridge
	aliceDeposits, err = testSuite.DB.BridgeTransfers.L1BridgeDepositsByAddress(aliceAddr, "", 1)
	require.NoError(t, err)
	require.Nil(t, aliceDeposits.Deposits[0].L1BridgeDeposit.CrossDomainMessageHash)
}

func TestE2EBridgeTransfersCursoredDeposits(t *testing.T) {
	testSuite := createE2ETestSuite(t)

	l1StandardBridge, err := bindings.NewL1StandardBridge(testSuite.OpCfg.L1Deployments.L1StandardBridgeProxy, testSuite.L1Client)
	require.NoError(t, err)
	optimismPortal, err := bindings.NewOptimismPortal(testSuite.OpCfg.L1Deployments.OptimismPortalProxy, testSuite.L1Client)
	require.NoError(t, err)

	aliceAddr := testSuite.OpCfg.Secrets.Addresses().Alice
	l1Opts, err := bind.NewKeyedTransactorWithChainID(testSuite.OpCfg.Secrets.Alice, testSuite.OpCfg.L1ChainIDBig())
	require.NoError(t, err)

	// Deposit 1/2/3 ETH (second deposit via the optimism portal)
	var depositReceipts [3]*types.Receipt
	for i := 0; i < 3; i++ {
		var depositTx *types.Transaction
		l1Opts.Value = big.NewInt(int64((i + 1)) * params.Ether)
		if i != 1 {
			depositTx, err = transactions.PadGasEstimate(l1Opts, 1.1, func(opts *bind.TransactOpts) (*types.Transaction, error) { return l1StandardBridge.Receive(opts) })
			require.NoError(t, err)
		} else {
			depositTx, err = transactions.PadGasEstimate(l1Opts, 1.1, func(opts *bind.TransactOpts) (*types.Transaction, error) { return optimismPortal.Receive(opts) })
			require.NoError(t, err)
		}

		depositReceipt, err := wait.ForReceiptOK(context.Background(), testSuite.L1Client, depositTx.Hash())
		require.NoError(t, err, fmt.Sprintf("failed on deposit %d", i))
		depositReceipts[i] = depositReceipt
	}

	// wait for processor catchup of the latest tx
	require.NoError(t, wait.For(context.Background(), 500*time.Millisecond, func() (bool, error) {
		l1Header := testSuite.Indexer.BridgeProcessor.LastL1Header
		return l1Header != nil && l1Header.Number.Uint64() >= depositReceipts[2].BlockNumber.Uint64(), nil
	}))

	// Get All
	aliceDeposits, err := testSuite.DB.BridgeTransfers.L1BridgeDepositsByAddress(aliceAddr, "", 3)
	require.NotNil(t, aliceDeposits)
	require.NoError(t, err)
	require.Len(t, aliceDeposits.Deposits, 3)
	require.False(t, aliceDeposits.HasNextPage)

	// Respects Limits & Supplied Cursors
	aliceDeposits, err = testSuite.DB.BridgeTransfers.L1BridgeDepositsByAddress(aliceAddr, "", 2)
	require.NotNil(t, aliceDeposits)
	require.NoError(t, err)
	require.Len(t, aliceDeposits.Deposits, 2)
	require.True(t, aliceDeposits.HasNextPage)

	aliceDeposits, err = testSuite.DB.BridgeTransfers.L1BridgeDepositsByAddress(aliceAddr, aliceDeposits.Cursor, 1)
	require.NoError(t, err)
	require.NotNil(t, aliceDeposits)
	require.Len(t, aliceDeposits.Deposits, 1)
	require.False(t, aliceDeposits.HasNextPage)

	// Returns the results in the right order
	aliceDeposits, err = testSuite.DB.BridgeTransfers.L1BridgeDepositsByAddress(aliceAddr, "", 3)
	require.NotNil(t, aliceDeposits)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		deposit := aliceDeposits.Deposits[i]

		// DESCENDING order
		require.Equal(t, depositReceipts[2-i].TxHash, deposit.L1TransactionHash)
		require.Equal(t, int64(3-i)*params.Ether, deposit.L1BridgeDeposit.Tx.Amount.Int64())
	}
}

func TestE2EBridgeTransfersStandardBridgeETHWithdrawal(t *testing.T) {
	testSuite := createE2ETestSuite(t)

	optimismPortal, err := bindings.NewOptimismPortal(testSuite.OpCfg.L1Deployments.OptimismPortalProxy, testSuite.L1Client)
	require.NoError(t, err)
	l2StandardBridge, err := bindings.NewL2StandardBridge(predeploys.L2StandardBridgeAddr, testSuite.L2Client)
	require.NoError(t, err)

	// 1 ETH transfer
	aliceAddr := testSuite.OpCfg.Secrets.Addresses().Alice
	l2Opts, err := bind.NewKeyedTransactorWithChainID(testSuite.OpCfg.Secrets.Alice, testSuite.OpCfg.L2ChainIDBig())
	require.NoError(t, err)
	l2Opts.Value = big.NewInt(params.Ether)

	// Ensure L1 has enough funds for the withdrawal by depositing an equal amount into the OptimismPortal
	l1Opts, err := bind.NewKeyedTransactorWithChainID(testSuite.OpCfg.Secrets.Alice, testSuite.OpCfg.L1ChainIDBig())
	require.NoError(t, err)
	l1Opts.Value = l2Opts.Value
	depositTx, err := optimismPortal.Receive(l1Opts)
	require.NoError(t, err)
	_, err = wait.ForReceiptOK(context.Background(), testSuite.L1Client, depositTx.Hash())
	require.NoError(t, err)

	// (1) Test Withdrawal Initiation
	withdrawTx, err := l2StandardBridge.Withdraw(l2Opts, predeploys.LegacyERC20ETHAddr, l2Opts.Value, 200_000, []byte{byte(1)})
	require.NoError(t, err)
	withdrawReceipt, err := wait.ForReceiptOK(context.Background(), testSuite.L2Client, withdrawTx.Hash())
	require.NoError(t, err)

	// wait for processor catchup
	require.NoError(t, wait.For(context.Background(), 500*time.Millisecond, func() (bool, error) {
		l2Header := testSuite.Indexer.BridgeProcessor.LastL2Header
		return l2Header != nil && l2Header.Number.Uint64() >= withdrawReceipt.BlockNumber.Uint64(), nil
	}))

	aliceWithdrawals, err := testSuite.DB.BridgeTransfers.L2BridgeWithdrawalsByAddress(aliceAddr, "", 3)
	require.NoError(t, err)
	require.Len(t, aliceWithdrawals.Withdrawals, 1)
	require.Equal(t, withdrawTx.Hash().String(), aliceWithdrawals.Withdrawals[0].L2TransactionHash.String())

	msgPassed, err := withdrawals.ParseMessagePassed(withdrawReceipt)
	require.NoError(t, err)
	withdrawalHash, err := withdrawals.WithdrawalHash(msgPassed)
	require.NoError(t, err)

	withdrawal := aliceWithdrawals.Withdrawals[0].L2BridgeWithdrawal
	require.Equal(t, withdrawalHash, withdrawal.TransactionWithdrawalHash)
	require.Equal(t, predeploys.LegacyERC20ETHAddr, withdrawal.TokenPair.LocalTokenAddress)
	require.Equal(t, predeploys.LegacyERC20ETHAddr, withdrawal.TokenPair.RemoteTokenAddress)
	require.Equal(t, uint64(params.Ether), withdrawal.Tx.Amount.Uint64())
	require.Equal(t, aliceAddr, withdrawal.Tx.FromAddress)
	require.Equal(t, aliceAddr, withdrawal.Tx.ToAddress)
	require.Equal(t, byte(1), withdrawal.Tx.Data[0])

	// StandardBridge flows through the messenger. We remove the first two
	// bytes of the nonce dedicated to the version. nonce == 0 (first message)
	require.NotNil(t, withdrawal.CrossDomainMessageHash)

	crossDomainBridgeMessage, err := testSuite.DB.BridgeMessages.L2BridgeMessage(*withdrawal.CrossDomainMessageHash)
	require.NoError(t, err)
	require.Nil(t, crossDomainBridgeMessage.RelayedMessageEventGUID)

	// (2) Test Withdrawal Proven/Finalized. Test the sql join queries to populate the right transaction
	require.Empty(t, aliceWithdrawals.Withdrawals[0].ProvenL1TransactionHash)
	require.Empty(t, aliceWithdrawals.Withdrawals[0].FinalizedL1TransactionHash)

	// wait for processor catchup
	proveReceipt, finalizeReceipt := op_e2e.ProveAndFinalizeWithdrawal(t, *testSuite.OpCfg, testSuite.L1Client, testSuite.OpSys.EthInstances["sequencer"], testSuite.OpCfg.Secrets.Alice, withdrawReceipt)
	require.NoError(t, wait.For(context.Background(), 500*time.Millisecond, func() (bool, error) {
		l1Header := testSuite.Indexer.BridgeProcessor.LastFinalizedL1Header
		return l1Header != nil && l1Header.Number.Uint64() >= finalizeReceipt.BlockNumber.Uint64(), nil
	}))

	aliceWithdrawals, err = testSuite.DB.BridgeTransfers.L2BridgeWithdrawalsByAddress(aliceAddr, "", 100)
	require.NoError(t, err)
	require.Equal(t, proveReceipt.TxHash, aliceWithdrawals.Withdrawals[0].ProvenL1TransactionHash)
	require.Equal(t, finalizeReceipt.TxHash, aliceWithdrawals.Withdrawals[0].FinalizedL1TransactionHash)
	require.Equal(t, withdrawReceipt.BlockHash, aliceWithdrawals.Withdrawals[0].L2BlockHash)

	crossDomainBridgeMessage, err = testSuite.DB.BridgeMessages.L2BridgeMessage(*withdrawal.CrossDomainMessageHash)
	require.NoError(t, err)
	require.NotNil(t, crossDomainBridgeMessage)
	require.NotNil(t, crossDomainBridgeMessage.RelayedMessageEventGUID)
}

func TestE2EBridgeTransfersL2ToL1MessagePasserETHReceive(t *testing.T) {
	testSuite := createE2ETestSuite(t)
	optimismPortal, err := bindings.NewOptimismPortal(testSuite.OpCfg.L1Deployments.OptimismPortalProxy, testSuite.L1Client)
	require.NoError(t, err)
	l2ToL1MessagePasser, err := bindings.NewOptimismPortal(predeploys.L2ToL1MessagePasserAddr, testSuite.L2Client)
	require.NoError(t, err)

	// 1 ETH transfer
	aliceAddr := testSuite.OpCfg.Secrets.Addresses().Alice
	l2Opts, err := bind.NewKeyedTransactorWithChainID(testSuite.OpCfg.Secrets.Alice, testSuite.OpCfg.L2ChainIDBig())
	require.NoError(t, err)
	l2Opts.Value = big.NewInt(params.Ether)

	// Ensure L1 has enough funds for the withdrawal by depositing an equal amount into the OptimismPortal
	l1Opts, err := bind.NewKeyedTransactorWithChainID(testSuite.OpCfg.Secrets.Alice, testSuite.OpCfg.L1ChainIDBig())
	require.NoError(t, err)
	l1Opts.Value = l2Opts.Value
	depositTx, err := optimismPortal.Receive(l1Opts)
	require.NoError(t, err)
	_, err = wait.ForReceiptOK(context.Background(), testSuite.L1Client, depositTx.Hash())
	require.NoError(t, err)

	// (1) Test Withdrawal Initiation
	l2ToL1MessagePasserWithdrawTx, err := l2ToL1MessagePasser.Receive(l2Opts)
	require.NoError(t, err)
	l2ToL1WithdrawReceipt, err := wait.ForReceiptOK(context.Background(), testSuite.L2Client, l2ToL1MessagePasserWithdrawTx.Hash())
	require.NoError(t, err)

	// wait for processor catchup
	require.NoError(t, wait.For(context.Background(), 500*time.Millisecond, func() (bool, error) {
		l2Header := testSuite.Indexer.BridgeProcessor.LastL2Header
		return l2Header != nil && l2Header.Number.Uint64() >= l2ToL1WithdrawReceipt.BlockNumber.Uint64(), nil
	}))

	aliceWithdrawals, err := testSuite.DB.BridgeTransfers.L2BridgeWithdrawalsByAddress(aliceAddr, "", 100)
	require.NoError(t, err)
	require.Len(t, aliceWithdrawals.Withdrawals, 1)
	require.Equal(t, l2ToL1MessagePasserWithdrawTx.Hash().String(), aliceWithdrawals.Withdrawals[0].L2TransactionHash.String())

	msgPassed, err := withdrawals.ParseMessagePassed(l2ToL1WithdrawReceipt)
	require.NoError(t, err)
	withdrawalHash, err := withdrawals.WithdrawalHash(msgPassed)
	require.NoError(t, err)

	withdrawal := aliceWithdrawals.Withdrawals[0].L2BridgeWithdrawal
	require.Equal(t, withdrawalHash, withdrawal.TransactionWithdrawalHash)
	require.Equal(t, predeploys.LegacyERC20ETHAddr, withdrawal.TokenPair.LocalTokenAddress)
	require.Equal(t, predeploys.LegacyERC20ETHAddr, withdrawal.TokenPair.RemoteTokenAddress)
	require.Equal(t, uint64(params.Ether), withdrawal.Tx.Amount.Uint64())
	require.Equal(t, aliceAddr, withdrawal.Tx.FromAddress)
	require.Equal(t, aliceAddr, withdrawal.Tx.ToAddress)
	require.Len(t, withdrawal.Tx.Data, 0)

	// withdrawal was not sent through the cross domain messenger
	require.Nil(t, withdrawal.CrossDomainMessageHash)

	// (2) Test Withdrawal Proven/Finalized. Test the sql join queries to populate the right transaction
	require.Empty(t, aliceWithdrawals.Withdrawals[0].ProvenL1TransactionHash)
	require.Empty(t, aliceWithdrawals.Withdrawals[0].FinalizedL1TransactionHash)

	// wait for processor catchup
	proveReceipt, finalizeReceipt := op_e2e.ProveAndFinalizeWithdrawal(t, *testSuite.OpCfg, testSuite.L1Client, testSuite.OpSys.EthInstances["sequencer"], testSuite.OpCfg.Secrets.Alice, l2ToL1WithdrawReceipt)
	require.NoError(t, wait.For(context.Background(), 500*time.Millisecond, func() (bool, error) {
		l1Header := testSuite.Indexer.BridgeProcessor.LastFinalizedL1Header
		return l1Header != nil && l1Header.Number.Uint64() >= finalizeReceipt.BlockNumber.Uint64(), nil
	}))

	aliceWithdrawals, err = testSuite.DB.BridgeTransfers.L2BridgeWithdrawalsByAddress(aliceAddr, "", 100)
	require.NoError(t, err)
	require.Equal(t, proveReceipt.TxHash, aliceWithdrawals.Withdrawals[0].ProvenL1TransactionHash)
	require.Equal(t, finalizeReceipt.TxHash, aliceWithdrawals.Withdrawals[0].FinalizedL1TransactionHash)
}

func TestE2EBridgeTransfersCursoredWithdrawals(t *testing.T) {
	testSuite := createE2ETestSuite(t)

	l2StandardBridge, err := bindings.NewL2StandardBridge(predeploys.L2StandardBridgeAddr, testSuite.L2Client)
	require.NoError(t, err)
	l2ToL1MP, err := bindings.NewOptimismPortal(predeploys.L2ToL1MessagePasserAddr, testSuite.L2Client)
	require.NoError(t, err)

	aliceAddr := testSuite.OpCfg.Secrets.Addresses().Alice
	l2Opts, err := bind.NewKeyedTransactorWithChainID(testSuite.OpCfg.Secrets.Alice, testSuite.OpCfg.L2ChainIDBig())
	require.NoError(t, err)

	// Withdraw 1/2/3 ETH (second deposit via the l2ToL1MP). We dont ever finalize these withdrawals on
	// L1 so we dont have to worry about funding the OptimismPortal contract with ETH
	var withdrawReceipts [3]*types.Receipt
	for i := 0; i < 3; i++ {
		var withdrawTx *types.Transaction
		l2Opts.Value = big.NewInt(int64((i + 1)) * params.Ether)
		if i != 1 {
			withdrawTx, err = transactions.PadGasEstimate(l2Opts, 1.1, func(opts *bind.TransactOpts) (*types.Transaction, error) { return l2StandardBridge.Receive(opts) })
			require.NoError(t, err)
		} else {
			withdrawTx, err = transactions.PadGasEstimate(l2Opts, 1.1, func(opts *bind.TransactOpts) (*types.Transaction, error) { return l2ToL1MP.Receive(opts) })
			require.NoError(t, err)
		}

		withdrawReceipt, err := wait.ForReceiptOK(context.Background(), testSuite.L2Client, withdrawTx.Hash())
		require.NoError(t, err, fmt.Sprintf("failed on withdrawal %d", i))
		withdrawReceipts[i] = withdrawReceipt
	}

	// wait for processor catchup of the latest tx
	require.NoError(t, wait.For(context.Background(), 500*time.Millisecond, func() (bool, error) {
		l2Header := testSuite.Indexer.BridgeProcessor.LastL2Header
		return l2Header != nil && l2Header.Number.Uint64() >= withdrawReceipts[2].BlockNumber.Uint64(), nil
	}))

	// Get All
	aliceWithdrawals, err := testSuite.DB.BridgeTransfers.L2BridgeWithdrawalsByAddress(aliceAddr, "", 100)
	require.NotNil(t, aliceWithdrawals)
	require.NoError(t, err)
	require.Len(t, aliceWithdrawals.Withdrawals, 3)
	require.False(t, aliceWithdrawals.HasNextPage)

	// Respects Limits & Supplied Cursors
	aliceWithdrawals, err = testSuite.DB.BridgeTransfers.L2BridgeWithdrawalsByAddress(aliceAddr, "", 2)
	require.NotNil(t, aliceWithdrawals)
	require.NoError(t, err)
	require.Len(t, aliceWithdrawals.Withdrawals, 2)
	require.True(t, aliceWithdrawals.HasNextPage)

	aliceWithdrawals, err = testSuite.DB.BridgeTransfers.L2BridgeWithdrawalsByAddress(aliceAddr, aliceWithdrawals.Cursor, 1)
	require.NotNil(t, aliceWithdrawals)
	require.NoError(t, err)
	require.Len(t, aliceWithdrawals.Withdrawals, 1)
	require.False(t, aliceWithdrawals.HasNextPage)

	// Returns the results in the right order
	aliceWithdrawals, err = testSuite.DB.BridgeTransfers.L2BridgeWithdrawalsByAddress(aliceAddr, "", 100)
	require.NotNil(t, aliceWithdrawals)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		withdrawal := aliceWithdrawals.Withdrawals[i]

		// DESCENDING order
		require.Equal(t, withdrawReceipts[2-i].TxHash, withdrawal.L2TransactionHash)
		require.Equal(t, int64(3-i)*params.Ether, withdrawal.L2BridgeWithdrawal.Tx.Amount.Int64())
	}
}

func TestClientBridgeFunctions(t *testing.T) {
	testSuite := createE2ETestSuite(t)

	// (1) Generate contract bindings for the L1 and L2 standard bridges
	optimismPortal, err := bindings.NewOptimismPortal(testSuite.OpCfg.L1Deployments.OptimismPortalProxy, testSuite.L1Client)
	require.NoError(t, err)
	l2ToL1MessagePasser, err := bindings.NewOptimismPortal(predeploys.L2ToL1MessagePasserAddr, testSuite.L2Client)
	require.NoError(t, err)

	// (2) Create test actors that will deposit and withdraw using the standard bridge
	aliceAddr := testSuite.OpCfg.Secrets.Addresses().Alice
	bobAddr := testSuite.OpCfg.Secrets.Addresses().Bob
	malAddr := testSuite.OpCfg.Secrets.Addresses().Mallory

	type actor struct {
		addr common.Address
		priv *ecdsa.PrivateKey
	}

	mintSum := bigint.Zero
	withdrawSum := bigint.Zero

	actors := []actor{
		{
			addr: aliceAddr,
			priv: testSuite.OpCfg.Secrets.Alice,
		},
		{
			addr: bobAddr,
			priv: testSuite.OpCfg.Secrets.Bob,
		},
		{
			addr: malAddr,
			priv: testSuite.OpCfg.Secrets.Mallory,
		},
	}

	// (3) Iterate over each actor and deposit / withdraw
	for _, actor := range actors {
		l2Opts, err := bind.NewKeyedTransactorWithChainID(actor.priv, testSuite.OpCfg.L2ChainIDBig())
		require.NoError(t, err)
		l2Opts.Value = big.NewInt(params.Ether)

		// (3.a) Deposit user funds into L2 via OptimismPortal contract
		l1Opts, err := bind.NewKeyedTransactorWithChainID(actor.priv, testSuite.OpCfg.L1ChainIDBig())
		require.NoError(t, err)
		l1Opts.Value = l2Opts.Value
		depositTx, err := optimismPortal.Receive(l1Opts)
		require.NoError(t, err)
		depositReceipt, err := wait.ForReceiptOK(context.Background(), testSuite.L1Client, depositTx.Hash())
		require.NoError(t, err)

		mintSum = new(big.Int).Add(mintSum, depositTx.Value())

		// (3.b) Initiate withdrawal transaction via L2ToL1MessagePasser contract
		l2ToL1MessagePasserWithdrawTx, err := l2ToL1MessagePasser.Receive(l2Opts)
		require.NoError(t, err)
		l2ToL1WithdrawReceipt, err := wait.ForReceiptOK(context.Background(), testSuite.L2Client, l2ToL1MessagePasserWithdrawTx.Hash())
		require.NoError(t, err)

		// (3.c) wait for indexer processor to catchup with the L1 & L2 block containing the deposit & withdrawal tx
		require.NoError(t, wait.For(context.Background(), 500*time.Millisecond, func() (bool, error) {
			l1Header := testSuite.Indexer.BridgeProcessor.LastL1Header
			l2Header := testSuite.Indexer.BridgeProcessor.LastL2Header
			seenL2 := l2Header != nil && l2Header.Number.Uint64() >= l2ToL1WithdrawReceipt.BlockNumber.Uint64()
			seenL1 := l1Header != nil && l1Header.Number.Uint64() >= depositReceipt.BlockNumber.Uint64()
			return seenL1 && seenL2, nil
		}))

		withdrawSum = new(big.Int).Add(withdrawSum, l2ToL1MessagePasserWithdrawTx.Value())

		// (3.d) Ensure that withdrawal and deposit txs are retrievable via API
		deposits, err := testSuite.Client.GetAllDepositsByAddress(actor.addr)
		require.NoError(t, err)
		require.Len(t, deposits, 1)
		require.Equal(t, depositTx.Hash().String(), deposits[0].L1TxHash)

		withdrawals, err := testSuite.Client.GetAllWithdrawalsByAddress(actor.addr)
		require.NoError(t, err)
		require.Len(t, withdrawals, 1)
		require.Equal(t, l2ToL1MessagePasserWithdrawTx.Hash().String(), withdrawals[0].TransactionHash)

	}

	// (4) Ensure that supply assessment is correct
	assessment, err := testSuite.Client.GetSupplyAssessment()
	require.NoError(t, err)

	mintFloat, _ := mintSum.Float64()
	require.Equal(t, mintFloat, assessment.L1DepositSum)

	withdrawFloat, _ := withdrawSum.Float64()
	require.Equal(t, withdrawFloat, assessment.L2WithdrawalSum)

}
