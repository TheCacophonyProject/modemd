module github.com/TheCacophonyProject/modemd

go 1.12

require (
	github.com/TheCacophonyProject/event-reporter/v3 v3.3.0
	github.com/TheCacophonyProject/go-config v1.8.3
	github.com/TheCacophonyProject/salt-updater v0.4.0
	github.com/alexflint/go-arg v1.4.2
	github.com/godbus/dbus v4.1.0+incompatible
	periph.io/x/periph v3.6.8+incompatible
)

replace periph.io/x/periph => github.com/TheCacophonyProject/periph v2.1.1-0.20200615222341-6834cd5be8c1+incompatible
