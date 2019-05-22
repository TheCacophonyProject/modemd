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
	"io/ioutil"

	yaml "gopkg.in/yaml.v2"
)

type ModemdConfig struct {
	ModemsConfig      []ModemConfig `yaml:"modems"`
	TestHosts         []string      `yaml:"test-hosts"`
	TestInterval      int           `yaml:"test-interval"`
	PowerPin          string        `yaml:"power-pin"`
	InitialOnTime     int           `yaml:"initial-on-time"`
	FindModemTime     int           `yaml:"find-modem-time"`
	ConnectionTimeout int           `yaml:"connection-timeout"`
	PingWaitTime      int           `yaml:"ping-wait-time"`
	PingRetries       int           `yaml:"ping-retries"`
	RequestOnTime     int           `yaml:"request-on-time"`
}

type ModemConfig struct {
	Name          string `yaml:"name"`
	Netdev        string `yaml:"netdev"`
	VendorProduct string `yaml:"vendor-product"`
}

func ParseModemdConfig(filename string) (*ModemdConfig, error) {
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	conf := &ModemdConfig{}
	if err := yaml.Unmarshal(buf, &conf); err != nil {
		return nil, err
	}
	return conf, nil
}
