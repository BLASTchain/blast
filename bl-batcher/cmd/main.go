package main

import (
	"context"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/BLASTchain/blast/bl-batcher/batcher"
	"github.com/BLASTchain/blast/bl-batcher/flags"
	"github.com/BLASTchain/blast/bl-batcher/metrics"
	opservice "github.com/BLASTchain/blast/bl-service"
	"github.com/BLASTchain/blast/bl-service/cliapp"
	oplog "github.com/BLASTchain/blast/bl-service/log"
	"github.com/BLASTchain/blast/bl-service/metrics/doc"
	"github.com/BLASTchain/blast/bl-service/opio"
	"github.com/ethereum/go-ethereum/log"
)

var (
	Version   = "v0.10.14"
	GitCommit = ""
	GitDate   = ""
)

func main() {
	oplog.SetupDefaults()

	app := cli.NewApp()
	app.Flags = cliapp.ProtectFlags(flags.Flags)
	app.Version = opservice.FormatVersion(Version, GitCommit, GitDate, "")
	app.Name = "bl-batcher"
	app.Usage = "Batch Submitter Service"
	app.Description = "Service for generating and submitting L2 tx batches to L1"
	app.Action = cliapp.LifecycleCmd(batcher.Main(Version))
	app.Commands = []*cli.Command{
		{
			Name:        "doc",
			Subcommands: doc.NewSubcommands(metrics.NewMetrics("default")),
		},
	}

	ctx := opio.WithInterruptBlocker(context.Background())
	err := app.RunContext(ctx, os.Args)
	if err != nil {
		log.Crit("Application failed", "message", err)
	}
}
