package main

import (
	"fmt"
	"log"
	"log/syslog"
	"os"
	"sync"
)

var (
	Logger    = log.New(os.Stdout, "log-shuttle: ", log.LstdFlags)
	ErrLogger = log.New(os.Stderr, "log-shuttle: ", log.LstdFlags)
)

const (
	VERSION = "0.9.4"
)

func MakeBasicBits(config ShuttleConfig) (reader Reader, deliverableBatches chan Batch, programStats *ProgramStats, bWaiter, oWaiter *sync.WaitGroup) {
	programStats = NewProgramStats(config.StatsAddr, config.StatsBuff)
	programStats.Listen()
	go EmitStats(programStats, config.StatsInterval, config.StatsSource)

	deliverableBatches = make(chan Batch, config.BackBuff)
	// Start outlets, then batches (reverse of Shutdown)
	reader = NewReader(config.FrontBuff, programStats.Input)
	oWaiter = StartOutlets(config, programStats.Drops, programStats.Lost, programStats.Input, deliverableBatches)
	bWaiter = StartBatchers(config, programStats.Drops, programStats.Input, reader.Outbox, deliverableBatches)
	return
}

func Shutdown(deliverableLogs chan LogLine, stats chan NamedValue, deliverableBatches chan Batch, bWaiter *sync.WaitGroup, oWaiter *sync.WaitGroup) {
	close(deliverableLogs)    // Close the log line channel, all of the batchers will stop once they are done
	bWaiter.Wait()            // Wait for them to be done
	close(deliverableBatches) // Close the batch channel, all of the outlets will stop once they are done
	oWaiter.Wait()            // Wait for them to be done
	close(stats)              // Close the stats channel to shut down any goroutines using it
}

func main() {
	var config ShuttleConfig
	var err error

	config.ParseFlags()

	if config.PrintVersion {
		fmt.Println(VERSION)
		os.Exit(0)
	}

	if !config.UseStdin() {
		ErrLogger.Fatalln("No stdin detected.")
	}

	if config.LogToSyslog {
		Logger, err = syslog.NewLogger(syslog.LOG_INFO|syslog.LOG_SYSLOG, 0)
		if err != nil {
			log.Fatalf("Unable to setup syslog logger: %s\n", err)
		}
		ErrLogger, err = syslog.NewLogger(syslog.LOG_ERR|syslog.LOG_SYSLOG, 0)
		if err != nil {
			log.Fatalf("Unable to setup syslog error logger: %s\n", err)
		}
	}

	reader, deliverableBatches, programStats, batchWaiter, outletWaiter := MakeBasicBits(config)

	// Blocks until closed
	reader.Read(os.Stdin)

	// Shutdown everything else.
	Shutdown(reader.Outbox, programStats.Input, deliverableBatches, batchWaiter, outletWaiter)
}
