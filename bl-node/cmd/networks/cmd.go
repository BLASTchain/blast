package networks

import (
	"encoding/json"
	"errors"
	"fmt"

	opnode "github.com/BLASTchain/blast/bl-node"
	"github.com/BLASTchain/blast/bl-node/flags"
	oplog "github.com/BLASTchain/blast/bl-service/log"
	"github.com/urfave/cli/v2"
)

var Subcommands = []*cli.Command{
	{
		Name:  "dump-rollup-config",
		Usage: "Dumps network configs",
		Flags: []cli.Flag{
			flags.Network,
		},
		Action: func(ctx *cli.Context) error {
			logCfg := oplog.ReadCLIConfig(ctx)
			logger := oplog.NewLogger(oplog.AppOut(ctx), logCfg)

			network := ctx.String(flags.Network.Name)
			if network == "" {
				return errors.New("must specify a network name")
			}

			rCfg, err := opnode.NewRollupConfig(logger, ctx)
			if err != nil {
				return err
			}

			out, err := json.MarshalIndent(rCfg, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(out))
			return nil
		},
	},
}
