package main

import (
	"fmt"
	"os/exec"
	"strings"
)

type Modem struct {
	Name          string
	Netdev        string
	VendorID      string
	ProductID     string
	ATReady       bool
	SimCardStatus SimCardStatus
	ATManager     *atManager
}

type SimCardStatus string

const (
	SimCardFinding SimCardStatus = "finding"
	SimCardReady   SimCardStatus = "ready"
	SimCardFailed  SimCardStatus = "failed"
)

// NewModem return a new modem from the config
func NewModem(config ModemConfig) *Modem {
	m := &Modem{
		Name:          config.Name,
		Netdev:        config.NetDev,
		VendorID:      config.VendorID,
		ProductID:     config.ProductID,
		SimCardStatus: SimCardFinding,
	}
	return m
}

// PingTest will try connecting to one of the provides hosts
func (m *Modem) PingTest(timeoutSec int, hosts []string) bool {
	for _, host := range hosts {
		cmd := exec.Command(
			"ping",
			"-I",
			m.Netdev,
			"-n",
			"-q",
			"-c1",
			fmt.Sprintf("-w%d", timeoutSec),
			host)
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	return false
}

// IsDefaultRoute will check if the USB modem is connected
func (m *Modem) IsDefaultRoute() (bool, error) {
	outByte, err := exec.Command("ip", "route").Output()
	if err != nil {
		return false, err
	}
	out := string(outByte)
	lines := strings.Split(out, "\n")
	search := fmt.Sprintf(" dev %s ", m.Netdev)
	for _, line := range lines {
		if strings.HasPrefix(line, "default") && strings.Contains(line, search) {
			return true, nil
		}
	}
	return false, nil
}

/*
func (m *Modem) WaitForConnection(timeout int) (bool, error) {
	start := time.Now()
	for {
		def, err := m.IsDefaultRoute()
		if err != nil {
			return false, err
		}
		if def {
			return true, nil
		}
		if time.Since(start) > time.Second*time.Duration(timeout) {
			return false, nil
		}
		time.Sleep(time.Second)
	}
}
*/
