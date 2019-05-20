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

import "time"

type ModemController struct {
	startTime     time.Time
	Modem         *Modem
	InitialOnTime int // Number of seconds the modem will be kept on for when first started

}

func (mc *ModemController) ShouldBeOff() bool {
	if mc.startTime.Add(time.Second * time.Duration(mc.InitialOnTime)).Before(time.Now()) {
		return true
	}

	// time from last ping

	return false
}
