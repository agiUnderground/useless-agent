module useless-agent

go 1.27.4

require (
	github.com/dblohm7/wingoes v0.0.0-20260526185140-fb298caac7ca // indirect
	github.com/ebitengine/purego v0.11.3 // indirect
	github.com/gen2brain/shm v0.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.4 // indirect
	github.com/jezek/xgb v1.3.1 // indirect
	github.com/jezek/xgbutil v0.0.0-20260124183602-9fd151d6a51a // indirect
	github.com/lufia/plan9stats v0.0.0-20261101121723-be84dd9509a0 // indirect
	github.com/power-devops/perfstat v0.0.0-20260916203055-22a1a467d9f0 // indirect
	github.com/shirou/gopsutil/v4 v4.26.10 // indirect
	github.com/tailscale/win v0.0.0-20260619195133-2d76c33a64c1 // indirect
	github.com/tidwall/gjson v1.20.5 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.3.0 // indirect
	github.com/tidwall/sjson v1.2.11 // indirect
	github.com/tklauser/go-sysconf v0.4.0 // indirect
	github.com/tklauser/numcpus v0.12.0 // indirect
	github.com/vcaesar/go-wayland v0.50.0 // indirect
	github.com/vcaesar/gops v0.43.0 // indirect
	github.com/vcaesar/imgo v0.44.0 // indirect
	github.com/vcaesar/keycode v0.20.0 // indirect
	github.com/vcaesar/screenshot v0.40.1 // indirect
	github.com/vcaesar/tt v0.40.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/exp v0.0.0-20261120182113-8801fbdfac81 // indirect
	golang.org/x/image v0.48.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
)

require (
	github.com/BurntSushi/xgb v0.0.0-20210121224620-deaf085860bc
	github.com/go-vgo/robotgo v1.3.8
	github.com/gorilla/websocket v1.5.3
	github.com/openai/openai-go v1.12.0
	github.com/otiai10/gosseract/v2 v2.4.1
	github.com/trustsight-io/deepseek-go v0.1.1
	internal/vision v1.0.0
	useless-agent/pkg/x11 v1.0.0
)

replace useless-agent/pkg/x11 => ./internal/x11

replace internal/vision => ./internal/vision
