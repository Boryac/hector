package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "Hector ResearchOS",
		Width:  720,
		Height: 560,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour:  &options.RGBA{R: 250, G: 250, B: 248, A: 1},
		StartHidden:       true, // 托盘常驻，窗口按需显示
		HideWindowOnClose: true, // 关窗只隐藏，不停后台
		OnStartup:         app.startup,
		Bind: []interface{}{
			app,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "hector-researchos-tray-v1",
			OnSecondInstanceLaunch: func(secondInstance options.SecondInstanceData) {
				// 重复启动：把已有实例的窗口带出来
				wailsruntime.WindowUnminimise(app.ctx)
				wailsruntime.WindowShow(app.ctx)
			},
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
