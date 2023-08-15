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
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	goconfig "github.com/TheCacophonyProject/go-config"
	"github.com/tarm/serial"
	"periph.io/x/periph/conn/gpio"
	"periph.io/x/periph/conn/gpio/gpioreg"
)

type ModemController struct {
	StartTime              time.Time
	Modem                  *Modem
	ModemsConfig           []goconfig.Modem
	TestHosts              []string
	TestInterval           time.Duration
	PowerPin               string
	InitialOnDuration      time.Duration
	FindModemDuration      time.Duration // Time in seconds after USB powered on for the modem to be found
	ConnectionTimeout      time.Duration // Time in seconds for modem to make a connection to the network
	PingWaitTime           time.Duration
	PingRetries            int
	RequestOnDuration      time.Duration // Time the modem will stay on in seconds after a request was made
	RetryInterval          time.Duration
	RetryFindModemInterval time.Duration
	MaxOffDuration         time.Duration
	MinConnDuration        time.Duration

	lastOnRequestTime    time.Time
	lastSuccessfulPing   time.Time
	lastFailedConnection time.Time
	lastFailedFindModem  time.Time
	connectedTime        time.Time
	onOffReason          string
}

func (mc *ModemController) NewOnRequest() {
	mc.lastOnRequestTime = time.Now()
}

func (mc *ModemController) FindModem() bool {
	timeout := time.After(mc.FindModemDuration)
	for {
		select {
		case <-timeout:
			return false
		case <-time.After(time.Second):
			for _, modemConfig := range mc.ModemsConfig {
				cmd := exec.Command("lsusb", "-d", modemConfig.VendorProductID)
				if err := cmd.Run(); err == nil {
					mc.Modem = NewModem(modemConfig)
					return true
				}
			}
		}
	}
}

func (mc *ModemController) TurnOnModem() error {
	c := &serial.Config{Name: "/dev/ttyUSB2", Baud: 115200}
	s, err := serial.OpenPort(c)
	if err != nil {
		return err
	}

	_, err = s.Write([]byte("AT\r"))
	if err != nil {
		return err
	}

	buf := make([]byte, 128)
	n, err := s.Read(buf)
	if err != nil {
		return err
	}

	fmt.Println(string(buf[:n]))
	return nil
}

func (mc *ModemController) SetModemPower(on bool) error {
	//TODO Change it to power on/off the USB modem through pin GPIO16, LOW is off.
	if err := setUSBPower(on); err != nil {
		return err
	}

	pin := gpioreg.ByName("GPIO22")
	if pin == nil {
		return fmt.Errorf("failed to init GPIO16 pin")
	}
	if on {
		if err := pin.Out(gpio.High); err != nil {
			return fmt.Errorf("failed to set modem power pin high: %v", err)
		}
	} else {
		if err := pin.Out(gpio.Low); err != nil {
			return fmt.Errorf("failed to set modem power pin high: %v", err)
		}
	}
	return nil
}

func setUSBPower(enabled bool) error {
	// TODO When powering on USB hub, power off the ethernet port as that is not needed still.
	var command string
	if enabled {
		command = "echo 1 | sudo tee /sys/devices/platform/soc/3f980000.usb/buspower"
	} else {
		command = "echo 0 | sudo tee /sys/devices/platform/soc/3f980000.usb/buspower"
	}

	cmd := exec.Command("bash", "-c", command)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to set USB power: %w", err)
	}
	return nil
}

func (mc *ModemController) CycleModemPower() error {
	if err := mc.SetModemPower(false); err != nil {
		return err
	}
	return mc.SetModemPower(true)
}

// WaitForConnection will return false if no connection is made before either
// it timeouts or the modem should no longer be powered.
func (mc *ModemController) WaitForConnection() (bool, error) {
	timeout := time.After(mc.ConnectionTimeout)
	for {
		select {
		case <-timeout:
			mc.lastFailedConnection = time.Now()
			return false, nil
		case <-time.After(time.Second):
			def, err := mc.Modem.IsDefaultRoute()
			if err != nil {
				mc.lastFailedConnection = time.Now()
				return false, err
			}
			if def && mc.PingTest() {
				return true, nil
			}
		}
	}
}

// ShouldBeOff will look at the following factors to determine if the modem should be off.
// - InitialOnTime: Modem should be on for a set amount of time at the start.
// - LastOnRequest: Check if the last "StayOn" request was less than 'RequestOnTime' ago.
// - OnWindow: //TODO
func (mc *ModemController) shouldBeOnWithReason() (bool, string) {
	if time.Since(mc.lastFailedFindModem) < mc.RetryFindModemInterval {
		return false, fmt.Sprintf("shouldn't retry finding modem for %v", mc.RetryFindModemInterval)
	}

	if time.Since(mc.lastFailedConnection) < mc.RetryInterval {
		return false, fmt.Sprintf("modem shouldn't retry connection for %v", mc.RetryInterval)
	}

	if time.Since(mc.StartTime) < mc.InitialOnDuration {
		return true, fmt.Sprintf("modem should be on for initial %v", mc.InitialOnDuration)
	}

	if time.Since(mc.lastOnRequestTime) < mc.RequestOnDuration {
		return true, fmt.Sprintf("modem should be on because of it being requested in the last %v", mc.RequestOnDuration)
	}

	if time.Since(mc.lastSuccessfulPing) > mc.MaxOffDuration {
		return true, fmt.Sprintf("modem should be on because modem has been off for over %s", mc.MaxOffDuration)
	}

	if time.Since(mc.connectedTime) < mc.MinConnDuration {
		return true, fmt.Sprintf("modem should be on because minimum connection duration is %v", mc.MinConnDuration)
	}

	if saltCommandsRunning() {
		return true, fmt.Sprintln("modem should be on because salt commands are running")
	}

	return false, "no reason the modem should be on"
}

func saltCommandsRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "salt-call", "--local", "saltutil.running")

	stdout, err := cmd.Output()
	if err != nil {
		log.Println(err)
		return false
	}

	return len(strings.Split(strings.TrimSpace(string(stdout)), "\n")) > 2
}

func (mc *ModemController) ShouldBeOn() bool {
	on, reason := mc.shouldBeOnWithReason()
	if mc.onOffReason != reason {
		mc.onOffReason = reason
		log.Println(reason)
	}
	return on
}

// WaitForNextPingTest will return false if when waiting ShouldBeOff returns
// true, otherwise will return true after waiting.
func (mc *ModemController) WaitForNextPingTest() bool {
	timeout := time.After(mc.TestInterval)
	for {
		select {
		case <-timeout:
			return true
		case <-time.After(time.Second):
			if !mc.ShouldBeOn() {
				return false
			}
		}
	}
}

func (mc *ModemController) PingTest() bool {
	seconds := int(mc.PingWaitTime / time.Second)
	return mc.Modem.PingTest(seconds, mc.PingRetries, mc.TestHosts)
}