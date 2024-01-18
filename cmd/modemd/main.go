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

package main

import (
	"fmt"
	"log"
	"time"

	"github.com/TheCacophonyProject/go-config"
	arg "github.com/alexflint/go-arg"
	"periph.io/x/periph/host"
)

func main() {
	err := runMain()
	if err != nil {
		log.Fatal(err)
	}
}

type Args struct {
	ConfigDir    string `arg:"-c,--config" help:"path to configuration directory"`
	Timestamps   bool   `arg:"-t,--timestamps" help:"include timestamps in log output"`
	RestartModem bool   `arg:"-r,--restart" help:"cycle the power to the USB port"`
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

func runMain() error {
	args := procArgs()
	if !args.Timestamps {
		log.SetFlags(0) // Removes default timestamp flag
	}
	log.Printf("Running version: %s", version)

	if _, err := host.Init(); err != nil {
		return err
	}

	conf, err := ParseModemdConfig(args.ConfigDir)
	if err != nil {
		return err
	}

	log.Printf("%+v\n", conf)

	m := []config.Modem{
		{Name: "Qualcomm", NetDev: "usb0", VendorProductID: "1e0e:9011"},
	}

	mc := ModemController{
		StartTime: time.Now(),
		//ModemsConfig:           conf.ModemsConfig,
		ModemsConfig:           m,
		TestHosts:              conf.TestHosts,
		TestInterval:           conf.TestInterval,
		PowerPin:               conf.PowerPin,
		InitialOnDuration:      conf.InitialOnDuration,
		FindModemDuration:      conf.FindModemDuration,
		ConnectionTimeout:      conf.ConnectionTimeout,
		PingWaitTime:           conf.PingWaitTime,
		PingRetries:            conf.PingRetries,
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

	for {
		if !mc.ShouldBeOn() {
			log.Println("Waiting until modem should be powered on.")
			for !mc.ShouldBeOn() {
				time.Sleep(5 * time.Second)
			}
		}
		if err := mc.SetModemPower(true); err != nil {
			return err
		}

		log.Println("Finding USB modem.")
		retries := 3
		for mc.ShouldBeOn() {
			if mc.FindModem() {
				log.Printf("Found modem %s.\n", mc.Modem.Name)
				usbMode, err := mc.IsInUSBMode()
				if err != nil {
					return err
				}
				if !usbMode {
					if err := mc.EnableUSBMode(); err != nil {
						return err
					}
				}
				break
			}
			retries--
			if retries < 1 {
				log.Println("Failed to find USB modem, will check again later.")
				mc.lastFailedFindModem = time.Now()
				if err := mc.SetModemPower(false); err != nil {
					return err
				}
				break
			} else {
				log.Printf("No USB modem found. Will cycle power %d more time(s) to find modem", retries)
			}
			mc.CycleModemPower()
		}

		if mc.Modem != nil {
			log.Println("Waiting for AT command to respond.")
			atCommandSuccess := false
			for i := 0; i < 20; i++ {
				_, err := mc.RunATCommand("AT")
				if err == nil {
					atCommandSuccess = true
					break
				}
				time.Sleep(time.Second)
			}
			if atCommandSuccess {
				log.Println("Got response from AT command.")
			} else {
				return fmt.Errorf("failed to get response from AT command")
			}
			time.Sleep(5 * time.Second) // Wait a little bit longer or else might get AT ERRORS

			if err := mc.DisableGPS(); err != nil {
				return err
			}

			connected, err := mc.WaitForConnection()
			if err != nil {
				return err
			}
			connectionsFirstPing := true
			if connected {
				log.Println("Modem has connected to a network.")
				mc.connectedTime = time.Now()
				for {
					if mc.PingTest() {
						mc.lastSuccessfulPing = time.Now()
						if connectionsFirstPing {
							connectionsFirstPing = false
							sendModemConnectedSignal() // This allows programs to trigger events when the modem connects.
						}
					} else {
						log.Println("ping test failed")
						mc.lastFailedConnection = time.Now()
						break
					}
					if !mc.WaitForNextPingTest() {
						break
					}
				}
			} else {
				log.Println("Modem failed to connect to a network.")
			}
		}

		log.Println("Powering off USB modem.")
		if err := mc.SetModemPower(false); err != nil {
			return err
		}
		mc.Modem = nil
	}
}
