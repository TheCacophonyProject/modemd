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
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/TheCacophonyProject/event-reporter/v3/eventclient"
	"github.com/TheCacophonyProject/go-utils/saltutil"
	"periph.io/x/periph/conn/gpio"
	"periph.io/x/periph/conn/gpio/gpioreg"
)

type ModemController struct {
	StartTime         time.Time
	Modem             *Modem
	ModemsConfig      []ModemConfig
	TestHosts         []string
	TestInterval      time.Duration
	PowerPin          string
	InitialOnDuration time.Duration
	FindModemDuration time.Duration // Time in seconds after USB powered on for the modem to be found
	ConnectionTimeout time.Duration // Time in seconds for modem to make a connection to the network
	PingWaitTime      time.Duration
	//PingRetries            int
	RequestOnDuration      time.Duration // Time the modem will stay on in seconds after a request was made
	RetryInterval          time.Duration
	RetryFindModemInterval time.Duration
	MaxOffDuration         time.Duration
	MinConnDuration        time.Duration

	lastOnRequestTime    time.Time
	lastSuccessfulPing   time.Time
	lastFailedConnection time.Time
	//lastFailedFindModem  time.Time
	connectedTime time.Time
	stayOnUntil   time.Time
	stayOffUntil  time.Time
	onOffReason   string
	IsPowered     bool

	failedToFindModem   bool
	failedToFindSimCard bool
}

const PinEnableModem = "GPIO22"
const PinPowerModem = "GPIO20"

func (mc *ModemController) NewOnRequest() {
	mc.lastOnRequestTime = time.Now()
}

func (mc *ModemController) StayOnUntil(onUntil time.Time) error {
	mc.stayOnUntil = onUntil
	mc.stayOffUntil = time.Time{}
	log.Println("dbus request to keep modem on until", onUntil.Format(time.DateTime))
	return nil
}

func (mc *ModemController) StayOffUntil(offUntil time.Time) error {
	mc.stayOffUntil = offUntil
	mc.stayOnUntil = time.Time{}
	return nil
}

/*
func (mc *ModemController) gpsEnabled() (bool, error) {
	out, err := mc.RunATCommand("AT+CGPS?")
	log.Println(out)
	return out == "+CGPS: 1,1", err
}
*/

/*
func (mc *ModemController) EnableGPS() error {
	enabled, err := mc.gpsEnabled()
	if err != nil {
		return err
	}
	if enabled {
		return nil
	}
	_, err = mc.RunATCommand("AT+CGPS=1")
	return err
}
*/

func (mc *ModemController) DisableGPS() error {
	_, err := mc.RunATCommand("AT+CGPS=0", 1000, 1)
	return err
}

/*
type gpsData struct {
	latitude    float64
	longitude   float64
	utcDateTime time.Time
	altitude    float64
	speed       float64
	course      float64
}

// Convert to a map that is compatible with DBus
func (g *gpsData) ToDBusMap() map[string]interface{} {
	return map[string]interface{}{
		"latitude":    g.latitude,
		"longitude":   g.longitude,
		"utcDateTime": g.utcDateTime.Format("2006-01-02 15:04:05"),
		"altitude":    g.altitude,
		"speed":       g.speed,
		"course":      g.course,
	}
}
*/

/*
func (mc *ModemController) GetGPSStatus() (*gpsData, error) {
	out, err := mc.RunATCommand("AT+CGPSINFO")
	if err != nil {
		return nil, err
	}
	//log.Println(out)
	out = strings.TrimSpace(out)
	out = strings.TrimPrefix(out, "+CGPSINFO:")
	out = strings.TrimSpace(out)
	parts := strings.Split(out, ",")
	if len(parts) < 8 {
		return nil, fmt.Errorf("invalid GPS format")
	}
	latRaw := parts[0]
	latNSRaw := parts[1]
	longRaw := parts[2]
	longEWRaw := parts[3]
	utcDateRaw := parts[4]
	utcTimeRaw := parts[5]
	altitudeRaw := parts[6]
	speedRaw := parts[7]
	courseRaw := parts[8]
	//log.Println("latRaw:", latRaw)
	//log.Println("latNSRaw:", latNSRaw)
	//log.Println("longRaw:", longRaw)
	//log.Println("longEW:", longEWRaw)
	//log.Println("utcDateRaw:", utcDateRaw)
	//log.Println("utcTimeRaw:", utcTimeRaw)
	//log.Println("altitude:", altitudeRaw)
	//log.Println("speed:", speedRaw)
	//log.Println("course:", courseRaw)

	if latRaw == "" {
		return nil, fmt.Errorf("no GPS data available")
	}
	if string(latRaw[4]) != "." {
		return nil, fmt.Errorf("invalid latitude")
	}
	latDeg, err := strconv.ParseFloat(latRaw[:2], 64)
	if err != nil {
		return nil, err
	}
	latMinute, err := strconv.ParseFloat(latRaw[2:], 64)
	if err != nil {
		return nil, err
	}
	latDeg += latMinute / 60
	if latNSRaw == "S" {
		latDeg *= -1
	} else if latNSRaw != "N" {
		return nil, fmt.Errorf("invalid latitude direction")
	}
	//log.Println("latDeg:", latDeg)

	if string(longRaw[5]) != "." {
		return nil, fmt.Errorf("invalid longitude")
	}
	longDeg, err := strconv.ParseFloat(longRaw[:3], 64)
	if err != nil {
		return nil, err
	}
	longMinute, err := strconv.ParseFloat(longRaw[3:], 64)
	if err != nil {
		return nil, err
	}
	longDeg += longMinute / 60
	if longEWRaw == "W" {
		latDeg *= -1
	} else if longEWRaw != "E" {
		return nil, fmt.Errorf("invalid longitude direction")
	}
	//log.Println("longDeg:", longDeg)

	const layout = "020106-150405.0" // format DDMMYY-hhmmss.s
	dateTime, err := time.Parse(layout, utcDateRaw+"-"+utcTimeRaw)
	//log.Println(dateTime.Local().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, err
	}

	altitude, err := strconv.ParseFloat(altitudeRaw, 64)
	if err != nil {
		return nil, err
	}
	//log.Println(altitude)

	speed, err := strconv.ParseFloat(speedRaw, 64)
	if err != nil {
		return nil, err
	}
	//log.Println(speed)

	var course float64
	if courseRaw != "" {
		course, err = strconv.ParseFloat(courseRaw, 64)
		if err != nil {
			return nil, err
		}
	}
	//log.Println(course)

	return &gpsData{
		latitude:    latDeg,
		longitude:   longDeg,
		utcDateTime: dateTime,
		altitude:    altitude,
		speed:       speed,
		course:      course,
	}, nil
}
*/

func (mc *ModemController) GetStatus() (map[string]interface{}, error) {
	status := make(map[string]interface{})
	status["timestamp"] = time.Now().Format(time.RFC1123Z)
	status["powered"] = mc.IsPowered
	status["onOffReason"] = mc.onOffReason
	status["failedToFindModem"] = mc.failedToFindModem
	status["failedToFindSimCard"] = mc.failedToFindSimCard

	if mc.Modem != nil {
		// Set details for modem
		modem := make(map[string]interface{})
		modem["name"] = mc.Modem.Name
		modem["netdev"] = mc.Modem.Netdev
		modem["vendor"] = mc.Modem.VendorID + ":" + mc.Modem.ProductID
		modem["atReady"] = mc.Modem.ATReady
		modem["connectedTime"] = mc.connectedTime.Format(time.RFC1123Z)
		if mc.Modem.ATReady {
			modem["voltage"] = valueOrErrorStr(mc.readVoltage())
			modem["temp"] = valueOrErrorStr(mc.readTemp())
			modem["manufacturer"] = valueOrErrorStr(mc.getManufacturer())
			modem["model"] = valueOrErrorStr(mc.getModel())
			modem["serial"] = valueOrErrorStr(mc.getSerialNumber())
			modem["apn"] = valueOrErrorStr(mc.getAPN())
		}
		status["modem"] = modem

		// Set details for signal
		if mc.Modem.ATReady {
			signal := make(map[string]interface{})
			signalStrength, bitErrorRate, signalStatus, err := mc.signalStrength()
			if err != nil {
				signal["strength"] = err.Error()
				signal["bitErrorRate"] = err.Error()
			} else {
				signal["strength"] = strconv.Itoa(signalStrength)   // Converting to string for compatibility reasons
				signal["bitErrorRate"] = strconv.Itoa(bitErrorRate) // Converting to string for compatibility reasons
			}
			signal["status"] = signalStatus
			provider, accessTechnology, err := mc.readProvider()
			if err != nil {
				signal["provider"] = err.Error()
				signal["accessTechnology"] = err.Error()
			} else {
				signal["provider"] = provider
				signal["accessTechnology"] = accessTechnology
			}
			status["signal"] = signal
		}

		// Set details for SIM card
		simCard := make(map[string]interface{})
		simCard["simCardStatus"] = mc.Modem.SimCardStatus
		if mc.Modem.SimCardStatus == SimCardReady {
			simCard["ICCID"] = valueOrErrorStr(mc.readSimICCID())
			simCard["provider"] = valueOrErrorStr(mc.readSimProvider())
		}
		status["simCard"] = simCard
	}

	return status, nil
}

func valueOrErrorStr(s interface{}, e error) interface{} {
	if e != nil {
		log.Println(e)
		return fmt.Sprintf("%s: %s", s, e.Error())
	}
	return s
}

func (mc *ModemController) readVoltage() (float64, error) {
	out, err := mc.RunATCommand("AT+CBC", 1000, 1)
	if err != nil {
		return 0, err
	}
	originalOutput := out

	// will be of format "+CBC: 3.305V"
	out = strings.TrimSpace(out)
	out = strings.TrimPrefix(out, "+CBC:")
	out = strings.TrimSuffix(out, "V")
	out = strings.TrimSpace(out)
	voltage, err := strconv.ParseFloat(out, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse voltage from output '%s': %v", originalOutput, err)
	}
	return voltage, nil
}

func (mc *ModemController) readProvider() (string, string, error) {
	//+COPS: 0,0,"Spark NZ Spark NZ",7
	out, err := mc.RunATCommand("AT+COPS?", 1000, 1)
	if err != nil {
		return "", "", err
	}
	originalOutput := out
	out = strings.TrimPrefix(out, "+COPS:")
	out = strings.TrimSpace(out)
	items := strings.Split(out, ",")
	if len(items) < 4 {
		return "", "", fmt.Errorf("invalid COPS format '%s'", originalOutput)
	}
	accessTechnologyCode, err := strconv.Atoi(items[3])
	if err != nil {
		return "", "", fmt.Errorf("invalid COPS format '%s' err: '%v'", originalOutput, err)
	}
	accessTechnology := "Unknown"
	switch accessTechnologyCode {
	case 0:
		accessTechnology = "GSM"
	case 1:
		accessTechnology = "GSM Compact"
	case 2:
		accessTechnology = "3G"
	case 7:
		accessTechnology = "4G"
	case 8:
		accessTechnology = "CDMA/HDR"
	}
	return strings.Trim(items[2], "\""), accessTechnology, nil
}

func (mc *ModemController) readSimICCID() (string, error) {
	out, err := mc.RunATCommand("AT+CICCID", 1000, 1)
	if err != nil {
		return "", err
	}
	out = strings.TrimPrefix(out, "+ICCID:")
	out = strings.TrimSpace(out)
	return out, nil
}

func (mc *ModemController) readTemp() (int, error) {
	out, err := mc.RunATCommand("AT+CPMUTEMP", 1000, 1)
	if err != nil {
		return 0, err
	}
	originalOutput := out
	out = strings.TrimPrefix(out, "+CPMUTEMP:")
	out = strings.TrimSpace(out)
	temp, err := strconv.Atoi(out)
	if err != nil {
		return 0, fmt.Errorf("invalid CPMUTEMP format '%s'", originalOutput)
	}
	return temp, nil
}

func (mc *ModemController) readSimProvider() (string, error) {
	out, err := mc.RunATCommand("AT+CSPN?", 1000, 1)
	if err != nil {
		return "", err
	}
	parseOut := strings.TrimSpace(out)
	parseOut = strings.TrimPrefix(parseOut, "+CSPN:")
	parseOut = strings.TrimSpace(parseOut)
	parts := strings.Split(parseOut, ",")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid CSPN format '%s'", out)
	}
	return strings.Trim(parts[0], "\""), nil
}

func (mc *ModemController) getManufacturer() (string, error) {
	out, err := mc.RunATCommand("AT+CGMI", 1000, 1)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (mc *ModemController) getModel() (string, error) {
	out, err := mc.RunATCommand("AT+CGMR", 1000, 1)
	if err != nil {
		return "", err
	}
	out = strings.TrimPrefix(out, "+CGMR:")
	out = strings.TrimSpace(out)
	return out, nil
}

func (mc *ModemController) getSerialNumber() (string, error) {
	out, err := mc.RunATCommand("AT+CGSN", 1000, 1)
	if err != nil {
		return "", err
	}
	out = strings.TrimSpace(out)
	return out, nil
}

// TODO Look into more functionality to add
// 3.2.4 AT+CSIM Generic SIM access
// 3.2.5 AT+CRSM Restricted SIM access
// 3.2.20 AT+SIMEI Set IMEI for the module
// 4.2.12 AT+CNBP Preferred band selection
// 4.2.15 AT+CNSMOD Show network system mode
// GPS only mode?
// 9.1 Overview of AT Commands for SMS Control
// Firmware upgrades?

func (mc *ModemController) getAPN() (string, error) {
	out, err := mc.RunATCommand("AT+CGDCONT?", 1000, 1)
	if err != nil {
		return "", err
	}
	out = strings.TrimPrefix(out, "+CGDCONT:")
	out = strings.TrimSpace(out)
	parts := strings.Split(out, ",")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid CGDCONT format %s", out)
	}
	apn := parts[2]
	apn = strings.TrimPrefix(apn, "\"")
	apn = strings.TrimSuffix(apn, "\"")
	return apn, nil
}

func (mc *ModemController) setAPN(apn string) error {
	_, err := mc.RunATCommand(fmt.Sprintf("AT+CGDCONT=1,\"IP\",\"%s\"", apn), 1000, 1)
	if err != nil {
		return err
	}
	readAPN, err := mc.getAPN()
	if err != nil {
		return err
	}
	if readAPN != apn {
		return fmt.Errorf("failed to set APN, APN is '%s' when it was set as '%s'", readAPN, apn)
	}
	return err
}

func (mc *ModemController) CheckSimCard() (string, error) {
	// Enable verbose error messages.
	_, err := mc.RunATCommand("AT+CMEE=2", 1000, 1)
	if err != nil {
		return "", err
	}
	out, err := mc.RunATCommand("AT+CPIN?", 1000, 1)
	if err != nil {
		return "", err
	}
	out = strings.TrimPrefix(out, "+CPIN:")
	out = strings.TrimSpace(out)
	return out, nil
}

func (mc *ModemController) signalStrength() (int, int, string, error) {
	out, err := mc.RunATCommand("AT+CSQ", 1000, 1)
	if err != nil {
		return 0, 0, "", err
	}
	out = strings.TrimPrefix(out, "+CSQ:")
	out = strings.TrimSpace(out)

	parts := strings.Split(out, ",")
	if len(parts) == 2 {
		signalStrength, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			log.Errorf("Failed to convert signal strength to int: %v, Output: %s", err, out)
			return 0, 0, "", err
		}
		bitErrorRate, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			log.Errorf("Failed to convert bit error rate to int: %v, Output: %s", err, out)
			return 0, 0, "", err
		}
		status := ""

		if signalStrength == 99 {
			status = "no signal"
			// TODO update what a "poor" signal is, could be needed to be increases to 15
		} else if (bitErrorRate > 0 && bitErrorRate != 99) || signalStrength < 15 {
			status = "poor"
		} else if signalStrength < 19 {
			status = "ok"
		} else {
			status = "good"
		}

		return signalStrength, bitErrorRate, status, nil
	} else {
		return 0, 0, "", fmt.Errorf("unable to read reception, '%s'", out)
	}
}

func (mc *ModemController) readBand() (string, error) {
	out, err := mc.RunATCommand("AT+CPSI?", 1000, 1)
	if err != nil {
		return "", err
	}
	//log.Println(string(out))
	parts := strings.Split(out, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "BAND") {
			return part, nil
		}
	}

	return "", err
}

func (mc *ModemController) RunATCommand(atCommand string, timeoutMsec int, attempts int) (string, error) {
	return mc.Modem.ATManager.request(atCommand, timeoutMsec, attempts)
	//_, out, err := mc.RunATCommandTotalOutput(atCommand, timeoutMsec, attempts)
	//return out, err
}

func (mc *ModemController) SetUSBMode(mode string) error {
	_, err := mc.RunATCommand(fmt.Sprintf("AT+CUSBPIDSWITCH=%s,1,1", mode), 5000, 20)
	if err != nil {
		return err
	}
	return nil
}

//AT+CUSBPIDSWITCH=9011,1,1
//AT+CUSBPIDSWITCH=9018,1,1
//AT+CUSBPIDSWITCH=9001,1,1

func (mc *ModemController) SetModemPower(on bool) error {
	pinEn := gpioreg.ByName(PinEnableModem)
	if pinEn == nil {
		return fmt.Errorf("failed to init GPIO22 pin")
	}
	pinPowerEn := gpioreg.ByName(PinPowerModem)
	if pinPowerEn == nil {
		return fmt.Errorf("failed to init GPIO20 pin")
	}
	if on {
		log.Println("Powering on USB modem")
		if err := pinEn.Out(gpio.High); err != nil {
			return fmt.Errorf("failed to set modem power pin high: %v", err)
		}
		if err := pinPowerEn.Out(gpio.High); err != nil {
			return fmt.Errorf("failed to enable power for the modem: %v", err)
		}
	} else {
		_, _ = mc.RunATCommand("AT+CPOF", 1000, 0)
		if mc.Modem != nil {
			mc.Modem.ATReady = false
		}
		log.Println("Triggering modem shutdown.")
		if err := pinEn.Out(gpio.Low); err != nil {
			return fmt.Errorf("failed to set modem power pin low: %v", err)
		}
		log.Println("Waiting 30 seconds for modem to shutdown.")
		time.Sleep(30 * time.Second)
		if mc.Modem != nil {
			out, err := exec.Command("lsusb").CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to check if modem is powered off: %v, output: %s", err, out)
			}
			if strings.Contains(string(out), mc.Modem.VendorID) {
				eventclient.AddEvent(eventclient.Event{
					Timestamp: time.Now().UTC(),
					Type:      "failed-modem-shutdown",
				})
				log.Printf("Modem is not shutting down, cutting power to modem anyway: %s", out)
			}
		}
		log.Println("Powering off modem.")
		if err := pinPowerEn.Out(gpio.Low); err != nil {
			return fmt.Errorf("failed to disable power for the modem: %v", err)
		}
	}
	if err := setUSBPower(on); err != nil {
		return err
	}
	mc.IsPowered = on
	return nil
}

func setUSBPower(enable bool) error {
	var writeVal []byte
	if enable {
		log.Println("Enabling USB power")
		writeVal = []byte("1")
	} else {
		log.Println("Disabling USB power")
		writeVal = []byte("0")
	}

	err := os.WriteFile("/sys/devices/platform/soc/3f980000.usb/buspower", writeVal, 0644)
	if err != nil {
		enDis := "disable"
		if enable {
			enDis = "enable"
		}
		return fmt.Errorf("failed to %s USB power: %s", enDis, err)
	}

	if enable {
		// Function to disable ethernet port. It will get enabled when the USB hub is turned on.
		// Turning it off will saves a bit of power.
		go func() {
			startTime := time.Now()
			// Wait for ethernet to be enabled.
			for {
				out, err := exec.Command("lsusb").CombinedOutput()
				if err != nil {
					log.Println("Failed to check if ethernet is enabled", err)
					return
				}
				if strings.Contains(string(out), "ID 0424:ec00") {
					log.Println("Ethernet is enabled")
					break
				}
				if time.Since(startTime) > 10*time.Second {
					return
				}
				time.Sleep(100 * time.Millisecond)
			}

			// Disable ethernet port.
			err = os.WriteFile("/sys/bus/usb/devices/1-1:1.0/1-1-port1/disable", []byte("1"), 0644) // (thanks uhubctl)
			if err != nil {
				log.Println("Failed to disable ethernet port", err)
			}
			log.Println("Disabled ethernet port")
		}()
	}
	return nil
}

func (mc *ModemController) CycleModemPower() error {
	if err := mc.SetModemPower(false); err != nil {
		return err
	}
	return mc.SetModemPower(true)
}

// ShouldBeOff will look at the following factors to determine if the modem should be off.
// - InitialOnTime: Modem should be on for a set amount of time at the start.
// - LastOnRequest: Check if the last "StayOn" request was less than 'RequestOnTime' ago.
// - OnWindow: //TODO
func (mc *ModemController) shouldBeOnWithReason() (bool, string) {
	if time.Now().Before(mc.stayOffUntil) {
		return false, fmt.Sprintf("Modem should be off because it was requested to stay off until %s.", mc.stayOffUntil.Format("2006-01-02 15:04:05"))
	}

	if time.Now().Before(mc.stayOnUntil) {
		return true, fmt.Sprintf("Modem should be on because it was requested to stay on until %s.", mc.stayOnUntil.Format("2006-01-02 15:04:05"))
	}

	if mc.failedToFindModem {
		return false, "Modem should be off because it could not be found on boot."
	}

	if mc.failedToFindSimCard {
		return false, "Modem should be off because it could not find a SIM card."
	}

	if mc.Modem != nil && mc.Modem.SimCardStatus == SimCardFailed {
		return false, "Modem should be off because it failed to find a SIM card."
	}

	if time.Since(mc.lastFailedConnection) < mc.RetryInterval {
		return false, fmt.Sprintf("Modem shouldn't retry connection for %v.", mc.RetryInterval)
	}

	if time.Since(mc.StartTime) < mc.InitialOnDuration {
		return true, fmt.Sprintf("Modem should be on for initial %v.", mc.InitialOnDuration)
	}

	if time.Since(mc.lastOnRequestTime) < mc.RequestOnDuration {
		return true, fmt.Sprintf("Modem should be on because of it being requested in the last %v.", mc.RequestOnDuration)
	}

	if time.Since(mc.lastSuccessfulPing) > mc.MaxOffDuration {
		return true, fmt.Sprintf("Modem should be on because modem has been off for over %s.", mc.MaxOffDuration)
	}

	if time.Since(mc.connectedTime) < mc.MinConnDuration {
		return true, fmt.Sprintf("Modem should be on because minimum connection duration is %v.", mc.MinConnDuration)
	}

	if mc.IsPowered && saltCommandsRunning() {
		return true, fmt.Sprintln("Modem should be on because salt commands are running.")
	}

	return false, "No reason the modem should be on."
}

func saltCommandsRunning() bool {
	// Check if minion_id file is present
	// If the file is not present then making the salt-call will make the minion_id file from the hostname, so just return false
	if _, err := os.Stat("/etc/salt/minion_id"); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if !saltutil.IsSaltIdSet() {
		return false
	}
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

func (mc *ModemController) PingTest(timeoutSec int) bool {
	return mc.Modem.PingTest(timeoutSec, mc.TestHosts)
}
