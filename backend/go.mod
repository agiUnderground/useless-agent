module useless-agent

go 1.25.2

require (
	github.com/BurntSushi/xgb v0.0.0-20210121224620-deaf085860bc
	github.com/go-vgo/robotgo v1.0.0
	github.com/gorilla/websocket v1.5.3
	github.com/otiai10/gosseract/v2 v2.4.1
	github.com/trustsight-io/deepseek-go v0.1.1
	golang.org/x/image v0.36.0 // indirect
)

require (
	github.com/dblohm7/wingoes v0.0.0-20250822163801-6d8e6105c62d // indirect
	github.com/ebitengine/purego v0.9.1 // indirect
	github.com/gen2brain/shm v0.1.1 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/jezek/xgb v1.3.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20251013123823-9fd1530e3ec3 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/robotn/xgb v0.10.0 // indirect
	github.com/robotn/xgbutil v0.10.0 // indirect
	github.com/shirou/gopsutil/v4 v4.26.1 // indirect
	github.com/tailscale/win v0.0.0-20250627215312-f4da2b8ee071 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/vcaesar/gops v0.41.0 // indirect
	github.com/vcaesar/imgo v0.41.0 // indirect
	github.com/vcaesar/keycode v0.10.1 // indirect
	github.com/vcaesar/screenshot v0.11.1 // indirect
	github.com/vcaesar/tt v0.20.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/exp v0.0.0-20260212183809-81e46e3db34a // indirect
	golang.org/x/sys v0.41.0 // indirect
)

require internal/vision v1.0.0

require (
	github.com/openai/openai-go v1.12.0
	useless-agent/pkg/x11 v1.0.0
)

replace useless-agent/pkg/x11 => ./internal/x11

replace internal/vision => ./internal/vision
