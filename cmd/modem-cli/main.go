package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/TheCacophonyProject/go-config"
	"github.com/TheCacophonyProject/go-utils/logging"
	modemcontroller "github.com/TheCacophonyProject/modemd/modem-controller"
	"github.com/alexflint/go-arg"
)

type Args struct {
	ConfigDir string           `arg:"-c,--config" help:"path to configuration directory"`
	ATCmd     *atSubcommand    `arg:"subcommand:AT" help:"send an AT command"`
	Power     *powerSubcommand `arg:"subcommand:power" help:"power control"`
	Status    *subcommand      `arg:"subcommand:status" help:"get modem status"`
	// TODO:
	// GPS: on, off, restart, log
	// Reception: log
	logging.LogArgs
}

type atSubcommand struct {
	Cmd string `arg:"required" help:"AT command to send"`
}

type powerSubcommand struct {
	On      *powerOffOnSubcommand `arg:"subcommand:on" help:"turn modem on"`
	Off     *powerOffOnSubcommand `arg:"subcommand:off" help:"turn modem off"`
	Restart *subcommand           `arg:"subcommand:restart" help:"restart modem"`
}

type powerOffOnSubcommand struct {
	Minutes int `arg:"required" help:"minutes to stay on"`
}

type subcommand struct {
}

func (Args) Version() string {
	return version
}

func procArgs() Args {
	args := Args{
		ConfigDir: config.DefaultConfigDir,
	}
	arg.MustParse(&args)
	return args
}

var version = "<not set>"
var log = logging.NewLogger("info")

func main() {
	err := runMain()
	if err != nil {
		log.Fatal(err)
	}
}

func runMain() error {
	args := procArgs()

	log = logging.NewLogger(args.LogLevel)

	log.Printf("Running version: %s", version)

	if args.ATCmd != nil {
		return runAT(args.ATCmd)
	} else if args.Power != nil {
		return runPower(args.Power)
	} else if args.Status != nil {
		return runStatus()
	}

	return nil
}

func runAT(args *atSubcommand) error {
	log.Printf("Running AT command: %s", args.Cmd)
	totalOut, out, err := modemcontroller.RunATCommand(args.Cmd)
	if err != nil {
		return fmt.Errorf("failed to run AT command: %w, output: %s", err, out)
	}
	log.Printf("Output: %s", out)
	log.Printf("Total output: %s", totalOut)
	return nil
}

func runPower(args *powerSubcommand) error {
	if args.On != nil {
		log.Println("Turning modem on for ", args.On.Minutes, " minutes")
		if err := modemcontroller.StayOnFor(args.On.Minutes); err != nil {
			return fmt.Errorf("failed to turn modem on: %w", err)
		}
	} else if args.Off != nil {
		log.Println("Turning modem off for ", args.Off.Minutes, " minutes")
		if err := modemcontroller.StayOffFor(args.Off.Minutes); err != nil {
			return fmt.Errorf("failed to turn modem off: %w", err)
		}
	} else if args.Restart != nil {
		log.Println("Restarting modem")
		log.Println("Powering off modem")
		if err := modemcontroller.StayOffFor(10); err != nil {
			return fmt.Errorf("failed to turn modem off: %w", err)
		}
		// Check state and wait for it to be off
		for {
			state, err := modemcontroller.GetModemStatus()
			if err != nil {
				return err
			}
			log.Println("Checking if powered off.")
			powered, ok := state["powered"]
			if !ok {
				printMap(state, "")
				return fmt.Errorf("failed to parse modem state")
			}
			if !powered.(bool) {
				break
			}

			time.Sleep(time.Second)

		}
		log.Println("Modem powered off, waiting 10 seconds before powering on.")
		time.Sleep(10 * time.Second)
		log.Println("Powering on modem")
		if err := modemcontroller.StayOnFor(10); err != nil {
			return fmt.Errorf("failed to turn modem on: %w", err)
		}
	}

	return nil
}

func runStatus() error {
	log.Println("Getting modem status")

	status, err := modemcontroller.GetModemStatus()
	if err != nil {
		return fmt.Errorf("failed to get modem status: %w", err)
	}
	printMap(status, "")
	return nil
}

func printMap(m map[string]interface{}, indent string) {
	// Collect keys and sort them, this is so when printing it out multiple times the order will stay the same.
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Iterate over sorted keys
	for _, key := range keys {
		value := m[key]
		switch v := value.(type) {
		case map[string]interface{}:
			fmt.Printf("%s%s:\n", indent, key)
			printMap(v, indent+"\t")
		default:
			fmt.Printf("%s%s: %v\n", indent, key, value)
		}
	}
}
