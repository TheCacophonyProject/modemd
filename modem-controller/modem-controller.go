package modemcontroller

import (
	"github.com/godbus/dbus"
)

const (
	dbusPath   = "/org/cacophony/modemd"
	dbusDest   = "org.cacophony.modemd"
	methodBase = "org.cacophony.modemd"
)

func GetModemStatus() (map[string]interface{}, error) {
	obj, err := getDbusObj()
	if err != nil {
		return nil, err
	}

	status := make(map[string]interface{})
	err = obj.Call(methodBase+".GetStatus", 0).Store(&status)
	return status, err
}

func RunATCommand(atCommand string) (string, string, error) {
	obj, err := getDbusObj()
	if err != nil {
		return "", "", err
	}
	var totalOut, out string

	err = obj.Call(methodBase+".RunATCommand", 0, atCommand).Store(&totalOut, &out)
	return totalOut, out, err
}

func StayOnFor(minutes int) error {
	obj, err := getDbusObj()
	if err != nil {
		return err
	}
	return obj.Call(methodBase+".StayOnFor", 0, minutes).Store()
}

func StayOffFor(minutes int) error {
	obj, err := getDbusObj()
	if err != nil {
		return err
	}
	return obj.Call(methodBase+".StayOffFor", 0, minutes).Store()
}

func getDbusObj() (dbus.BusObject, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, err
	}
	obj := conn.Object(dbusDest, dbusPath)
	return obj, nil
}
