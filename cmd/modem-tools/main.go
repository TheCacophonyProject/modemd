package main

import (
	"fmt"
	"os"

	"github.com/TheCacophonyProject/go-utils/logging"
	checkgps "github.com/TheCacophonyProject/modemd/internal/check-gps"
	modemcli "github.com/TheCacophonyProject/modemd/internal/modem-cli"
	"github.com/TheCacophonyProject/modemd/internal/modemd"
	receptionlogger "github.com/TheCacophonyProject/modemd/internal/reception-logger"
)

var log *logging.Logger

func main() {
	err := runMain()
	if err != nil {
		log.Fatal(err)
	}
}

var version = "<not set>"

func runMain() error {
	log = logging.NewLogger("info")
	if len(os.Args) < 2 {
		log.Info("Usage: tool <subcommand> [args]")
		return fmt.Errorf("no subcommand given")
	}

	subcommand := os.Args[1]
	args := os.Args[2:]

	var err error
	switch subcommand {
	case "check-gps":
		err = checkgps.Run(args, version)
	case "modem-cli":
		err = modemcli.Run(args, version)
	case "modemd":
		err = modemd.Run(args, version)
	case "reception-logger":
		err = receptionlogger.Run(args, version)
	default:
		err = fmt.Errorf("unknown subcommand: %s", subcommand)
	}

	return err
}
