package modemcontroller

import (
	"github.com/godbus/dbus"
)

const (
	dbusPath   = "/org/cacophony/modemd"
	dbusDest   = "org.cacophony.modemd"
	methodBase = "org.cacophony.modemd"
)

func GetModemStatus() error {
	obj, err := getDbusObj()
	if err != nil {
		return err
	}
	return obj.Call(methodBase+".GetStatus", 0).Store()
}

func getDbusObj() (dbus.BusObject, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, err
	}
	obj := conn.Object(dbusDest, dbusPath)
	return obj, nil
}
