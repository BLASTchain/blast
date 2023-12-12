package contracts

import (
	"context"
	"testing"

	"github.com/BLASTchain/blast/bl-bindings/bindings"
	"github.com/BLASTchain/blast/bl-challenger/game/fault/types"
	"github.com/BLASTchain/blast/bl-service/sources/batching"
	batchingTest "github.com/BLASTchain/blast/bl-service/sources/batching/test"
	"github.com/stretchr/testify/require"
)

func TestVMContract_Oracle(t *testing.T) {
	vmAbi, err := bindings.MIPSMetaData.GetAbi()
	require.NoError(t, err)

	stubRpc := batchingTest.NewAbiBasedRpc(t, vmAddr, vmAbi)
	vmContract, err := NewVMContract(vmAddr, batching.NewMultiCaller(stubRpc, batching.DefaultBatchSize))
	require.NoError(t, err)

	stubRpc.SetResponse(vmAddr, methodOracle, batching.BlockLatest, nil, []interface{}{oracleAddr})

	oracleContract, err := vmContract.Oracle(context.Background())
	require.NoError(t, err)
	tx, err := oracleContract.AddGlobalDataTx(&types.PreimageOracleData{
		OracleData: make([]byte, 20),
	})
	require.NoError(t, err)
	// This test doesn't care about all the tx details, we just want to confirm the contract binding is using the
	// correct address
	require.Equal(t, &oracleAddr, tx.To)
}
