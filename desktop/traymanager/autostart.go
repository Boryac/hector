package main

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// autostart.go — 当前用户级开机启动（HKCU Run 键）。
// 默认关闭；只影响当前用户，不注册系统服务、不写其它用户的启动配置。

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const runValueName = "Hector ResearchOS Tray"

// SetAutostart 开启/关闭开机自启（当前用户级）。
// 开启后系统启动时运行托盘程序；托盘负责检测后台服务状态，不重复启动。
func SetAutostart(enable bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开 HKCU Run 失败: %w", err)
	}
	defer k.Close()

	if !enable {
		if err := k.DeleteValue(runValueName); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}
	exe, err := osExecutable()
	if err != nil {
		return err
	}
	return k.SetStringValue(runValueName, `"`+exe+`"`)
}

// AutostartEnabled 查询当前状态。
func AutostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(runValueName)
	return err == nil && v != ""
}
