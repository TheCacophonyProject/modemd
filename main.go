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
	"os/exec"
	"time"

	arg "github.com/alexflint/go-arg"
	"periph.io/x/periph/conn/gpio"
	"periph.io/x/periph/conn/gpio/gpioreg"
	"periph.io/x/periph/host"
)

func main() {
	err := runMain()
	if err != nil {
		log.Fatal(err)
	}
}

type Args struct {
	ConfigFile   string `arg:"-c,--config" help:"path to configuration file"`
	Timestamps   bool   `arg:"-t,--timestamps" help:"include timestamps in log output"`
	RestartModem bool   `arg:"-r,--restart" help:"cycle the power to the USB port"`
}

func (Args) Version() string {
	return version
}

func procArgs() Args {
	var args Args
	args.ConfigFile = "/etc/cacophony/modemd.yaml"
	arg.MustParse(&args)
	return args
}

var (
	version = "<not set>"
)

func runMain() error {
	args := procArgs()
	if !args.Timestamps {
		log.SetFlags(0) // Removes default timestamp flag
	}
	log.Printf("running version: %s", version)

	log.Print("init gpio")
	if _, err := host.Init(); err != nil {
		return err
	}

	conf, err := ParseModemdConfig(args.ConfigFile)
	if err != nil {
		return err
	}

	log.Printf("%+v\n", conf)

	mc := ModemController{
		startTime:     time.Now(),
		InitialOnTime: 60,
	}

	if mc.ShouldBeOff() || args.RestartModem {
		setModemPower(false, conf.PowerPin)
	}

	for {
		if mc.ShouldBeOff() {
			log.Println("waiting until modem shoudl be powered on")
			for mc.ShouldBeOff() {
				time.Sleep(time.Second * 5)
			}
		}

		log.Println("powering on USB modem")
		setModemPower(true, conf.PowerPin)

		log.Println("finding USB modem")
		for {
			mc.Modem = findModem(60, conf.ModemsConfig)
			if mc.Modem != nil {
				log.Printf("found modem %s\n", mc.Modem.Name)
				break
			}
			log.Println("no USB modem found")
			cycleModemPower(conf.PowerPin)
		}

		log.Println("waiting for modem to connect to network")
		connected, err := mc.Modem.WaitForConnection(300)
		if err != nil {
			return err
		}
		if connected {
			log.Println("modem has connected to network")
			for {
				if mc.ShouldBeOff() {
					log.Println("modem should no longer be on")
					break
				}

				if mc.Modem.PingTest(5, 30, conf.TestHosts) {
					log.Println("ping test passed")
				} else {
					log.Println("ping test failed")
					break
				}

				newPingTime := time.Now().Add(time.Duration(conf.TestInterval) * time.Second)
				for newPingTime.After(time.Now()) {
					if mc.ShouldBeOff() {
						log.Println("modem should no longer be on")
						break
					}
					time.Sleep(time.Second * 5)
				}
			}

		} else {
			log.Println("modem failed to connect to netowrk")
		}
		mc.Modem = nil

		log.Println("powering off USB modem")
		setModemPower(false, conf.PowerPin)
		time.Sleep(time.Second * 5) // ensure modem is fully powered off
	}

	return nil
}

func findModem(timeout int, modemsConfig []ModemConfig) *Modem {
	start := time.Now()
	for {
		for _, modemConfig := range modemsConfig {
			cmd := exec.Command("lsusb", "-d", modemConfig.VendorProduct)
			if err := cmd.Run(); err == nil {
				return NewModem(modemConfig)
			}
		}
		if time.Now().Sub(start) > time.Second*time.Duration(timeout) {
			return nil
		}
		time.Sleep(time.Second)
	}
}

func cycleModemPower(pinName string) error {
	if err := setModemPower(false, pinName); err != nil {
		return err
	}
	time.Sleep(time.Second * 5)
	return setModemPower(true, pinName)
}

func setModemPower(on bool, pinName string) error {
	pin := gpioreg.ByName(pinName)
	if on {
		if err := pin.Out(gpio.High); err != nil {
			return fmt.Errorf("failed to set modem power pin high: %v", err)
		}
	} else {
		if err := pin.Out(gpio.Low); err != nil {
			return fmt.Errorf("failed to set modem power pin low: %v", err)
		}
	}
	return nil
}
