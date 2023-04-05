package cmd

import (
	"context"
	"fmt"
	"github.com/application-research/edge-ur/api"
	"github.com/application-research/edge-ur/core"
	"github.com/application-research/edge-ur/jobs"
	"github.com/spf13/viper"
	"github.com/urfave/cli/v2"
	"strconv"
	"time"
)

func DaemonCmd() []*cli.Command {
	// add a command to run API node
	var daemonCommands []*cli.Command

	daemonCmd := &cli.Command{
		Name:  "daemon",
		Usage: "Edge gateway daemon that allows users to upload and download data to/from the Filecoin network.",

		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: "repo",
			},
		},

		Action: func(c *cli.Context) error {

			repo := c.String("repo")

			if repo == "" {
				repo = ".whypfs"
			}

			ln, err := core.NewEdgeNode(context.Background(), repo)
			if err != nil {
				return err
			}

			//	launch the jobs
			go runProcessors(ln)

			// launch the API node
			api.InitializeEchoRouterConfig(ln)
			api.LoopForever()

			return nil
		},
	}

	// add commands.
	daemonCommands = append(daemonCommands, daemonCmd)

	return daemonCommands

}

func runProcessors(ln *core.LightNode) {

	// run the job every 10 seconds.
	//bucketAssignFreq, err := strconv.Atoi(viper.Get("BUCKET_ASSIGN_JOB_FREQ").(string))
	//carGeneFreq, err := strconv.Atoi(viper.Get("CAR_GENERATOR_PROCESS").(string))
	//uploadFreq, err := strconv.Atoi(viper.Get("UPLOAD_PROCESS").(string))
	dealCheckFreq, err := strconv.Atoi(viper.Get("DEAL_CHECK").(string))

	if err != nil {
		dealCheckFreq = 10
	}

	//bucketAssignFreqTick := time.NewTicker(time.Duration(bucketAssignFreq) * time.Second)
	//carGeneFreqTick := time.NewTicker(time.Duration(carGeneFreq) * time.Second)
	//uploadFreqTick := time.NewTicker(time.Duration(uploadFreq) * time.Second)
	dealCheckFreqTick := time.NewTicker(time.Duration(dealCheckFreq) * time.Second)

	for {
		select {
		case <-dealCheckFreqTick.C:
			go func() {
				dealCheck := jobs.NewDealChecker(ln)
				d := jobs.CreateNewDispatcher() // dispatch jobs
				d.AddJob(dealCheck)
				d.Start(1)

				for {
					if d.Finished() {
						fmt.Printf("All jobs finished.\n")
						break
					}
				}
			}()
		}
	}
}
