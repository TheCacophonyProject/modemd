module github.com/TheCacophonyProject/modemd

go 1.12

require (
	github.com/TheCacophonyProject/event-reporter/v3 v3.3.0
	github.com/TheCacophonyProject/go-config v1.6.3
	github.com/alexflint/go-arg v1.1.0
	github.com/godbus/dbus v0.0.0-20181101234600-2ff6f7ffd60f
	github.com/pelletier/go-toml v1.6.0 // indirect
	github.com/spf13/afero v1.2.2 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/spf13/viper v1.5.0 // indirect
	periph.io/x/periph v3.6.4+incompatible
)

replace periph.io/x/periph => github.com/TheCacophonyProject/periph v2.1.1-0.20200615222341-6834cd5be8c1+incompatible
