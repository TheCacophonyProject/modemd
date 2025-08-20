/*
modemd - Communicates with USB modems
Copyright (C) 2019, The Cacophony Project

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.
*/

package modemd

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/TheCacophonyProject/event-reporter/v3/eventclient"
	"github.com/TheCacophonyProject/go-config"
	"github.com/TheCacophonyProject/go-utils/logging"
	arg "github.com/alexflint/go-arg"
	"periph.io/x/periph/host"
)

type Args struct {
	ConfigDir    string `arg:"-c,--config" help:"path to configuration directory"`
	Timestamps   bool   `arg:"-t,--timestamps" help:"include timestamps in log output"`
	RestartModem bool   `arg:"-r,--restart" help:"cycle the power to the USB port"`
	logging.LogArgs
}

var version = "<not set>"
var log = logging.NewLogger("info")
var defaultArgs = Args{
	ConfigDir: config.DefaultConfigDir,
}

func (Args) Version() string {
	return version
}

const modemSetupSteps = 10

func procArgs(input []string) (Args, error) {
	args := defaultArgs

	parser, err := arg.NewParser(arg.Config{}, &args)
	if err != nil {
		return Args{}, err
	}
	err = parser.Parse(input)
	if errors.Is(err, arg.ErrHelp) {
		parser.WriteHelp(os.Stdout)
		os.Exit(0)
	}
	if errors.Is(err, arg.ErrVersion) {
		fmt.Println(version)
		os.Exit(0)
	}
	return args, err
}

func Run(inputArgs []string, ver string) error {
	version = ver
	args, err := procArgs(inputArgs)
	if err != nil {
		return fmt.Errorf("failed to parse args: %v", err)
	}
	log = logging.NewLogger(args.LogLevel)

	log.Infof("Running version: %s", version)

	if _, err := host.Init(); err != nil {
		return err
	}

	conf, err := ParseModemdConfig(args.ConfigDir)
	if err != nil {
		return err
	}

	log.Printf("%+v\n", conf)

	// We had issue when the raspberry pi was starting up and the modem was being powered
	// on at the same time when powered from a 1S Li-ion battery. This would sometimes cause
	// power supply issues causing odd modem issues, or get stuck in a boot loop.
	// This just waits for the RPI to be up for at least 10 seconds.
	rpiUptime, err := rpiUptime()
	log.Infof("RPI Uptime: %s", rpiUptime)
	if err != nil {
		log.Errorf("Failed to get rpi uptime: %s. Waiting 10 seconds", err)
		time.Sleep(10 * time.Second)
	}
	if rpiUptime < 2*time.Minute {
		log.Info("Camera has been on for less than 2 minutes. Waiting 10 seconds. This is to prevent power supply issues when the RPi is still starting up and starting the modem at the same time.")
		time.Sleep(10 * time.Second)
	}

	// For now we are just loading this one modem, ignoring the ones set in the config.
	m := []ModemConfig{
		{Name: "Qualcomm", NetDev: "usb0", VendorID: "1e0e", ProductID: "9018"},
	}

	mc := ModemController{
		StartTime: time.Now(),
		//ModemsConfig:           conf.ModemsConfig,
		ModemsConfig:      m,
		TestHosts:         conf.TestHosts,
		TestInterval:      conf.TestInterval,
		PowerPin:          conf.PowerPin,
		InitialOnDuration: conf.InitialOnDuration,
		FindModemDuration: conf.FindModemDuration,
		ConnectionTimeout: conf.ConnectionTimeout,
		PingWaitTime:      conf.PingWaitTime,
		//PingRetries:            conf.PingRetries,
		RequestOnDuration:      conf.RequestOnDuration,
		RetryInterval:          conf.RetryInterval,
		RetryFindModemInterval: conf.RetryFindModemInterval,
		MinConnDuration:        conf.MinConnDuration,
		MaxOffDuration:         conf.MaxOffDuration,
	}

	log.Println("Starting dbus service.")
	if err := startService(&mc); err != nil {
		return err
	}

	if !mc.ShouldBeOn() || args.RestartModem {
		if err := mc.SetModemPower(false); err != nil {
			return err
		}
	}

	// Setting the label for the main loop
MainModemLoop:
	for {
		// =========== Power off modem if it shouldn't be on then wait until it should be on ===========
		if !mc.ShouldBeOn() {
			log.Println("Powering off USB modem.")
			if err := mc.SetModemPower(false); err != nil {
				return err
			}
			mc.Modem = nil
			// Wait until the modem no longer should be off
			for !mc.ShouldBeOn() {
				time.Sleep(time.Second)
			}
		}

		// =========== Power on modem ===========
		printSetupStep(1, "Powering on USB modem.")
		if err := mc.SetModemPower(true); err != nil {
			return err
		}

		// =========== Finding modem ===========
		// Failed to find the modem so we shouldn't try to find it again.
		if mc.failedToFindModem {
			log.Info("Failed to find the USB modem. Not trying to find it again.")
			// Wait until the modem doesn't need to be on again.
			// We just wait here so the modem status can still be queried.
			for {
				if !mc.ShouldBeOn() {
					continue MainModemLoop
				}
				time.Sleep(time.Second)
			}
		}
		printSetupStep(2, "Finding USB modem.")
		findingModemTimeout := time.Now().Add(mc.FindModemDuration)
		productID := ""
	FindModemLoop:
		for {
			vendorProductIDs, err := getUSBVendorProductIDs()
			if err != nil {
				log.Errorf("Failed to list usb devices: %v", err)
			}

			// Loop through the different modems that we support (just the one for now) to see if we can find the modem
			for _, modemConfig := range mc.ModemsConfig {
				for _, vendorProductID := range vendorProductIDs {
					if modemConfig.VendorID == vendorProductID.VendorID {
						log.Infof("Found modem with vendorID '%s'", modemConfig.VendorID)
						mc.Modem = NewModem(modemConfig)
						productID = vendorProductID.ProductID
						break FindModemLoop
					}
				}
			}

			// Timeout for finding the modem through USB
			if time.Now().After(findingModemTimeout) {
				// Log the devices found on lsusb. This is simply to help debug modem issues.
				log.Infof("Failed to find modem in given time '%s', here are the usb devices on the system:", mc.FindModemDuration)
				out, err := exec.Command("lsusb").CombinedOutput()
				if err != nil {
					log.Errorf("Failed to list usb devices: %v", err)
				}
				for _, line := range strings.Split(string(out), "\n") {
					line := strings.TrimSpace(line)
					if line != "" {
						log.Infof("\t%s", line)
					}
				}

				// Set that it failed to find the modem and return to the start of the main loop.
				mc.failedToFindModem = true
				log.Println("Making noModemFound event.")
				err = eventclient.AddEvent(eventclient.Event{
					Timestamp: time.Now(),
					Type:      "noModemFound",
				})
				if err != nil {
					log.Errorf("Failed to make noModemFound event: %v", err)
				}
				continue MainModemLoop
			}
		}

		// ========== Checking for AT response from modem. =============
		printSetupStep(3, "Checking for AT response from modem.")
		checkATTimeout := time.Now().Add(time.Minute)
		mc.Modem.ATManager = newATManager()
		for {
			// Try to see if AT command is available yet
			_, err := mc.Modem.ATManager.request("AT", 1000, 0)
			if err == nil {
				log.Println("AT command responding.")
				mc.Modem.ATReady = true
				break
			}

			if time.Now().After(checkATTimeout) {
				log.Error("Failed to find AT command in given time.")
				log.Println("Making noModemATCommandResponse event.")
				err := eventclient.AddEvent(eventclient.Event{
					Timestamp: time.Now(),
					Type:      "noModemATCommandResponse",
				})
				if err != nil {
					log.Errorf("Failed to make noModemATCommandResponse event: %v", err)
				}
				// TODO, if AT command is not responding then what?
				// If it is in the correct USB mode then just continue? if not then how should I handle this?
				// If it is not in the correct mode or does't have the USB0 up and running then we should reset the modem?
				break // For now we just continue to the next step

			}
			time.Sleep(time.Second)
		}

		printSetupStep(4, "Disabling GPS.")
		err := mc.DisableGPS()
		if err != nil {
			log.Error("Failed to disable GPS: ", err)
			// Not a critical error so will continue to next step.
			// TODO, if disabling GPS failed, then we should try again?
		}

		// ========== Checking that the modem is in the correct mode ==========
		printSetupStep(5, "Checking that the modem is in the correct mode.")
		if productID == mc.Modem.ProductID {
			log.Infof("Modem is in the correct mode. '%s'", productID)
		} else {
			log.Infof("Modem is not in the correct mode. '%s' != '%s'", productID, mc.Modem.ProductID)
			log.Infof("Moving modem to the new mode '%s'", mc.Modem.ProductID)
			err := mc.SetUSBMode(mc.Modem.ProductID)
			if err != nil {
				log.Errorf("Failed to set USB mode: %v", err)
			}

			// Trigger reset with AT+CRESET
			_, err = mc.RunATCommand("AT+CRESET", 2000, 3)
			if err != nil {
				log.Errorf("Failed to reset modem: %v", err)
			}

			// Wait for modem to go offline then back online with the correct product ID
			modemOffBackUpTimeout := time.Now().Add(time.Minute)
			for {
				if time.Now().After(modemOffBackUpTimeout) {
					log.Error("Failed to find modem in given time after changing USB mode.")
					// TODO, how should I try to recover from this?
					continue MainModemLoop
				}

				vendorProductIDs, err := getUSBVendorProductIDs()
				if err != nil {
					log.Errorf("Failed to get USB vendor product IDs: %v", err)
					time.Sleep(time.Second)
					continue
				}
				for _, vendorProductID := range vendorProductIDs {
					if vendorProductID.VendorID == mc.Modem.VendorID && vendorProductID.ProductID == mc.Modem.ProductID {
						log.Infof("Modem is back online with correct product ID. '%s'", mc.Modem.ProductID)
						continue MainModemLoop
					}
				}
			}
		}

		// =========== Checking SIM card in modem ===========
		// If the modem failed to find a SIM card, then we shouldn't try to find it again.
		if mc.failedToFindSimCard {
			log.Info("Modem failed to find a SIM card. Will not try to find it again.")
			// We just wait here so the modem status can still be queried.
			for {
				if !mc.ShouldBeOn() {
					continue MainModemLoop
				}
				time.Sleep(time.Second)
			}
		}
		printSetupStep(6, "Checking SIM card.")
		for retries := 30; retries > 0; retries-- {
			simStatus, err := mc.CheckSimCard()
			if err == nil && simStatus == "READY" {
				mc.Modem.SimCardStatus = SimCardReady
				break
			}
			if err != nil {
				log.Errorf("Failed to check SIM card: %v", err)
				break
			}
			log.Infof("SIM card not ready. Will try %d more time(s) to find SIM card, current status: %s", retries, simStatus)
			time.Sleep(time.Second)
		}
		if mc.Modem.SimCardStatus != SimCardReady {
			mc.Modem.SimCardStatus = SimCardFailed
			makeModemEvent("noModemSimCard", &mc)
			mc.failedToFindSimCard = true
			continue MainModemLoop // Go back to start of main loop.
		}
		mc.failedToFindSimCard = false
		log.Info("SIM card ready.")

		// ========== Checking signal strength ===========
		printSetupStep(7, "Checking signal strength.")
		getSignalStrengthTimeout := time.Now().Add(2 * time.Minute)
		for {
			strengthStr, bitErrorRate, status, _ := mc.signalStrength()
			if strengthStr != 99 {
				log.Printf("Signal strength: %d", strengthStr)
				log.Printf("Bit error rate: %d", bitErrorRate)
				log.Printf("Signal status: %s", status)
				makeModemEvent("modemSignal", &mc)
				break
			}
			log.Debugf("Signal strength not found, waiting 3 seconds then looking again.")
			time.Sleep(3 * time.Second)

			if time.Now().After(getSignalStrengthTimeout) {
				log.Info("Timed out waiting for signal strength.")
				mc.lastFailedConnection = time.Now()
				makeModemEvent("noModemSignal", &mc)
				continue MainModemLoop
			}
		}

		// ========== Checking that the network is up ===========
		printSetupStep(8, "Checking that the network is up.")
		networkUpTimeout := time.Now().Add(2 * time.Minute)
		for {
			if time.Now().After(networkUpTimeout) {
				// Took too long to find the network, try from the start again.
				makeModemEvent("noModemNetwork", &mc)
				log.Errorf("Took too long to find the network.")
				continue MainModemLoop
			}

			iface, err := net.InterfaceByName(mc.Modem.Netdev)
			if err != nil {
				log.Debugf("Network interface not found, waiting a second then looking again. Error: %v", err)
				// Network is not up yet, wait a second then look again.
				time.Sleep(time.Second)
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				log.Errorf("Failed to get network addresses: %v", err)
				time.Sleep(time.Second)
				continue
			}
			if len(addrs) == 0 {
				log.Error("No network addresses found, waiting a second then looking again.")
				time.Sleep(time.Second)
				continue
			}
			for _, addr := range addrs {
				log.Infof("Network address: %s", addr.String())
			}
			break
		}

		// ============ Checking ping through the network ============
		printSetupStep(9, "Checking ping through the network.")
		pingingTimeout := time.Now().Add(mc.ConnectionTimeout)
		for {
			// Check if the modem should still be on.
			if !mc.ShouldBeOn() {
				log.Info("Canceling ping test as modem should be off.")
				continue MainModemLoop
			}

			if time.Now().After(pingingTimeout) {
				// Took too long to ping, try from the start again.
				makeModemEvent("noModemPing", &mc)
				log.Errorf("Took too long to ping.")
				mc.lastFailedConnection = time.Now()
				continue MainModemLoop
			}

			if mc.PingTest(5000) { // This ping test run the ping test through the modem, not the wifi if available.
				log.Info("Modem has connected to a network.")
				mc.connectedTime = time.Now()
				makeModemEvent("modemConnectedToNetwork", &mc)
				sendModemConnectedSignal() // This send a dbus signal that allows programs to trigger events when the modem connects.
				break
			} else {
				log.Infof("Ping test failed. Trying again until the %s timeout.", mc.ConnectionTimeout)
			}
		}

		// Network is all ready now I guess?

		// ========== Running regular ping tests =============
		log.Infof("Running ping tests every %s.", mc.TestInterval)
		pingFailCount := 0
		for {
			time.Sleep(mc.TestInterval)
			log.Debug("Running a regular ping test.")
			if mc.PingTest(5000) {
				mc.lastSuccessfulPing = time.Now()
				pingFailCount = 0
			} else {
				pingFailCount++
				log.Infof("Ping test failed %d times in a row.", pingFailCount)
			}

			if pingFailCount > 3 {
				log.Infof("Ping test failed %d times in a row. Reporting failure.", pingFailCount)
				mc.lastFailedConnection = time.Now()
				continue MainModemLoop
			}
		}
	}
}

func makeModemEvent(eventType string, mc *ModemController) {
	log.Printf("Making modem event '%s'.", eventType)
	signalStrength, bitErrorRate, status, err := mc.signalStrength()
	if err != nil {
		log.Printf("Failed to get signal strength: %s", err)
	}
	band, err := mc.readBand()
	if err != nil {
		log.Printf("Failed to get band: %s", err)
	}
	simStatus, err := mc.CheckSimCard()
	if err != nil {
		log.Printf("Failed to get sim status: %s", err)
	}
	apn, err := mc.getAPN()
	if err != nil {
		log.Printf("Failed to get apn: %s", err)
	}
	simProvider, err := mc.readSimProvider()
	if err != nil {
		log.Printf("Failed to get sim provider: %s", err)
	}
	provider, accessTechnology, err := mc.readProvider()
	if err != nil {
		log.Printf("Failed to get provider: %s", err)
	}
	iccid, err := mc.readSimICCID()
	if err != nil {
		log.Printf("Failed to get iccid: %s", err)
	}

	eventclient.AddEvent(eventclient.Event{
		Timestamp: time.Now(),
		Type:      eventType,
		Details: map[string]interface{}{
			"signalStatus":     status,
			"signalStrengthDB": bitErrorRate,
			"signalStrength":   signalStrength,
			"band":             band,
			"simStatus":        simStatus,
			"apn":              apn,
			"provider":         provider,
			"accessTechnology": accessTechnology,
			"simProvider":      simProvider,
			"iccid":            iccid,
		},
	})
}

func rpiUptime() (time.Duration, error) {
	var info syscall.Sysinfo_t
	if err := syscall.Sysinfo(&info); err != nil {
		return 0, err
	}
	return time.Duration(info.Uptime) * time.Second, nil
}

func printSetupStep(i int, text string) {
	log.Infof("Modem set up step (%d/%d): %s", i, modemSetupSteps, text)
}

type VendorProductID struct {
	VendorID  string
	ProductID string
}

func getUSBVendorProductIDs() ([]VendorProductID, error) {
	out, err := exec.Command("lsusb").Output()
	if err != nil {
		return nil, err
	}

	var vendorProductIDs []VendorProductID
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		// Lines look like:  "Bus 001 Device 006: ID 1e0e:9011 Qualcomm / Option"
		parts := strings.Fields(line)
		for i, tok := range parts {
			if tok == "ID" && i+1 < len(parts) {
				ids := strings.SplitN(parts[i+1], ":", 2)
				if len(ids) == 2 {
					vendorProductIDs = append(vendorProductIDs, VendorProductID{
						VendorID:  ids[0],
						ProductID: ids[1],
					})
				}
				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return vendorProductIDs, nil
}
