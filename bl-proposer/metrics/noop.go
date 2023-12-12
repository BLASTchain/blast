package metrics

import (
	"github.com/BLASTchain/blast/bl-service/eth"
	opmetrics "github.com/BLASTchain/blast/bl-service/metrics"
	txmetrics "github.com/BLASTchain/blast/bl-service/txmgr/metrics"
)

type noopMetrics struct {
	opmetrics.NoopRefMetrics
	txmetrics.NoopTxMetrics
}

var NoopMetrics Metricer = new(noopMetrics)

func (*noopMetrics) RecordInfo(version string) {}
func (*noopMetrics) RecordUp()                 {}

func (*noopMetrics) RecordL2BlocksProposed(l2ref eth.L2BlockRef) {}
