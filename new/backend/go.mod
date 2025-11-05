module useless-agent

go 1.24.0

require (
	github.com/BurntSushi/xgb v0.0.0-20210121224620-deaf085860bc
	github.com/go-vgo/robotgo v0.110.5
	github.com/gorilla/websocket v1.5.3
	github.com/kbinani/screenshot v0.0.0-20240820160931-a8a2c5d0e191
	github.com/otiai10/gosseract/v2 v2.4.1
)

require (
	github.com/dblohm7/wingoes v0.0.0-20240820181039-f2b84150679e // indirect
	github.com/ebitengine/purego v0.8.0 // indirect
	github.com/gen2brain/shm v0.1.1 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/jezek/xgb v1.1.1 // indirect
	github.com/lufia/plan9stats v0.0.0-20240909124753-873cd0166683 // indirect
	github.com/lxn/win v0.0.0-20210218163916-a377121e959e // indirect
	github.com/otiai10/gosseract v2.2.1+incompatible // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/robotn/xgb v0.10.0 // indirect
	github.com/robotn/xgbutil v0.10.0 // indirect
	github.com/shirou/gopsutil/v4 v4.24.9 // indirect
	github.com/stretchr/testify v1.10.0 // indirect
	github.com/tailscale/win v0.0.0-20240926211701-28f7e73c7afb // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.9.0 // indirect
	github.com/vcaesar/gops v0.40.0 // indirect
	github.com/vcaesar/imgo v0.40.2 // indirect
	github.com/vcaesar/keycode v0.10.1 // indirect
	github.com/vcaesar/tt v0.20.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/exp v0.0.0-20241004190924-225e2abe05e6 // indirect
	golang.org/x/image v0.23.0 // indirect
	golang.org/x/net v0.46.0 // indirect
	golang.org/x/sys v0.37.0 // indirect
)

replace internal/vision => ./internal/vision

replace useless-agent/pkg/x11 => ./pkg/x11
