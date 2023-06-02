package modemlistener

import (
	"fmt"

	"github.com/godbus/dbus"
)

const (
	DBusPath      = "/org/cacophony/modemd"
	DBusInterface = "org.cacophony.modemd"
)

type ModemSignal struct {
	State bool
}

// GetModemConnectedSignalListener returns a channel that listens for the "ModemConnected" signal
// on the DBus system bus. The function returns an error if it fails to establish the connection.
//
// Return:
// - chan bool: A channel that receives a boolean value when the "ModemConnected" signal is detected.
// - error: An error if the function fails to establish the connection.
func GetModemConnectedSignalListener() (chan bool, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, err
	}

	rule := fmt.Sprintf("type='signal',interface='%s',path='%s'", DBusInterface, DBusPath)
	call := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule)
	if call.Err != nil {
		return nil, call.Err
	}

	modemSignals := make(chan *dbus.Signal, 10)
	conn.Signal(modemSignals)

	modemConnectedSignals := make(chan bool, 10)
	go func() {
		for v := range modemSignals {
			if v.Path == dbus.ObjectPath(DBusPath) && v.Name == DBusInterface+".ModemConnected" {
				modemConnectedSignals <- true
			}
		}
	}()

	return modemConnectedSignals, nil
}
