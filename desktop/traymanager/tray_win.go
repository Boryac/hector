package main

// tray_win.go — 自研 win32 系统托盘（Shell_NotifyIcon + TrackPopupMenu）。
//
// 为什么不用 Wails 自带托盘：wails v2.17.1 的 options.App 不含 SystemTray 字段
// （全模块检索 0 命中），v3 尚未达到 RC1 稳定标准。按任务允许的降级路径，
// 采用"Wails 窗口 + 原生 win32 托盘"组合：托盘运行在独立消息循环线程上，
// 与 Wails 窗口生命周期互不影响；菜单点击经 channel 派发到主逻辑。

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmAppTray       = 0x8000 + 1 // WM_APP+1 托盘回调消息
	nimAdd          = 0
	nimModify       = 1
	nimDelete       = 2
	nifMessage      = 0x01
	nifIcon         = 0x02
	nifTip          = 0x04
	imageIcon       = 1
	loadFromFile    = 0x00000010
	trackReturnCMD  = 0x0100
	trackPopupMenu  = 0x0002 // 右键弹出
	tpmAlignBottom  = 0x0020
	mfString        = 0x00000000
	mfSeparator     = 0x00000800
	mfGrayed        = 0x00000001
	csHREDRAW       = 0x0002
	csVREDRAW       = 0x0001
	wmCreate        = 0x0001
	wmDestroy       = 0x0002
	wmClose         = 0x0010
	wmCommand       = 0x0111
	wmContextmenu   = 0x007B
	msgOnlyWindow   = ^uintptr(2) // HWND_MESSAGE = (HWND)-3，64 位必须全宽
	messageOnlyName = "HectorResearchOSTrayWnd"
	windowClassName = "HectorResearchOSTrayClass"
)

type notifyIconData struct {
	CbSize           uint32
	HWnd             windows.HWND
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            windows.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     windows.Handle
}

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     windows.Handle
	HIcon         windows.Handle
	HCursor       windows.Handle
	HbrBackground windows.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       windows.Handle
}

type point struct{ X, Y int32 }
type msg struct {
	HWnd     windows.HWND
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

var (
	shell32           = windows.NewLazySystemDLL("shell32.dll")
	user32            = windows.NewLazySystemDLL("user32.dll")
	procShellNotify   = shell32.NewProc("Shell_NotifyIconW")
	procTrackPopup    = user32.NewProc("TrackPopupMenu")
	procCreatePopup   = user32.NewProc("CreatePopupMenu")
	procAppendMenu    = user32.NewProc("AppendMenuW")
	procDestroyMenu   = user32.NewProc("DestroyMenu")
	procSetForeground = user32.NewProc("SetForegroundWindow")
	procGetCursorPos  = user32.NewProc("GetCursorPos")
	procPostMessage   = user32.NewProc("PostMessageW")
	procLoadIcon      = user32.NewProc("LoadImageW")
)

var kernel32 = windows.NewLazySystemDLL("kernel32.dll")

// TrayCommand 托盘菜单命令。
type TrayCommand int

const (
	CmdOpenUI TrayCommand = iota + 1
	CmdStart
	CmdStop
	CmdRestart
	CmdStatus
	CmdOpenLogs
	CmdOpenData
	CmdSettings
	CmdExit
)

// TrayCallbacks 托盘动作的实现方（由 app 注入）。
type TrayCallbacks struct {
	Status func() (running bool, healthy bool) // 打开菜单时实时刷新
	On     func(TrayCommand)                   // 命令派发
}

type tray struct {
	proc    *os.Process
	cb      TrayCallbacks
	hwnd    windows.HWND
	hIcon   windows.Handle
	added   bool
	mu      sync.Mutex
	stopped bool
}

var currentTray *tray

// trayWndProc 消息回调。
func trayWndProc(hwnd windows.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmAppTray:
		switch lParam & 0xFFFF {
		case 0x0202: // WM_LBUTTONUP：双击/单击托盘打开 UI
			currentTray.dispatch(CmdOpenUI)
		case 0x0205, 0x007B: // WM_RBUTTONUP / WM_CONTEXTMENU：弹菜单
			currentTray.showMenu()
		}
		return 0
	case wmDestroy:
		currentTray.removeIcon()
		return 0
	}
	defProc, _, _ := user32.NewProc("DefWindowProcW").Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return defProc
}

// RunTray 启动托盘（阻塞在独立 goroutine 的消息循环上，调用方自行 go）。
func RunTray(cb TrayCallbacks) error {
	t := &tray{cb: cb}
	currentTray = t

	hInst, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)

	clsName, _ := windows.UTF16PtrFromString(windowClassName)
	var wc wndClassEx
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.LpfnWndProc = syscall.NewCallback(trayWndProc)
	wc.HInstance = windows.Handle(hInst)
	wc.LpszClassName = clsName
	proc, _, _ := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))
	if proc == 0 {
		return fmt.Errorf("RegisterClassExW 失败")
	}

	clsPtr, _ := windows.UTF16PtrFromString(windowClassName)
	namePtr, _ := windows.UTF16PtrFromString(messageOnlyName)
	hwndRaw, _, _ := user32.NewProc("CreateWindowExW").Call(
		0, uintptr(unsafe.Pointer(clsPtr)), uintptr(unsafe.Pointer(namePtr)),
		0, 0, 0, 0, 0, uintptr(msgOnlyWindow), 0, uintptr(hInst), 0)
	if hwndRaw == 0 {
		return fmt.Errorf("CreateWindowExW(message-only) 失败")
	}
	t.hwnd = windows.HWND(hwndRaw)

	// 图标：优先 exe 内嵌资源（wails 打包的 APPICON = 资源 ID 3），失败再取 exe 同目录 tray.ico。
	var hIcon uintptr
	procLoadIcon.Call(uintptr(hInst), 3, imageIcon, 0, 0, 0) // LR_DEFAULTSIZE 未用，固定 16x16 需小图
	if hi, _, _ := procLoadIcon.Call(uintptr(hInst), 3, imageIcon, 16, 16, 0); hi != 0 {
		hIcon = hi
	} else if exe, err := osExecutable(); err == nil {
		icoPath, _ := windows.UTF16PtrFromString(exe[:len(exe)-len(`.exe`)] + `.ico`)
		if hi, _, _ := procLoadIcon.Call(0, uintptr(unsafe.Pointer(icoPath)), imageIcon|loadFromFile, 16, 16, 0); hi != 0 {
			hIcon = hi
		}
	}
	t.hIcon = windows.Handle(hIcon)

	if err := t.addIcon(); err != nil {
		return err
	}

	var m msg
	for {
		r, _, _ := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return nil // WM_QUIT 或错误 → 退出消息循环
		}
		user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&m)))
		user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&m)))
		t.mu.Lock()
		stopped := t.stopped
		t.mu.Unlock()
		if stopped {
			return nil
		}
	}
}

func (t *tray) dispatch(c TrayCommand) {
	if t.cb.On != nil {
		go t.cb.On(c)
	}
}

func (t *tray) addIcon() error {
	nid := notifyIconData{
		CbSize:           uint32(unsafe.Sizeof(notifyIconData{})),
		HWnd:             t.hwnd,
		UID:              1,
		UFlags:           nifMessage | nifTip,
		UCallbackMessage: wmAppTray,
	}
	copyTip(&nid.SzTip, "Hector ResearchOS — 初始化向导")
	if t.hIcon != 0 {
		nid.UFlags |= nifIcon
		nid.HIcon = t.hIcon
	}
	r, _, err := procShellNotify.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	if r == 0 {
		return fmt.Errorf("Shell_NotifyIcon(NIM_ADD) 失败: %v", err)
	}
	t.added = true
	return nil
}

func (t *tray) removeIcon() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.added {
		return
	}
	nid := notifyIconData{CbSize: uint32(unsafe.Sizeof(notifyIconData{})), HWnd: t.hwnd, UID: 1}
	procShellNotify.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	t.added = false
}

// UpdateTip 更新悬浮提示（如「Hector ResearchOS — 运行中」）。
func (t *tray) UpdateTip(tip string) {
	if !t.added {
		return
	}
	nid := notifyIconData{
		CbSize: uint32(unsafe.Sizeof(notifyIconData{})), HWnd: t.hwnd, UID: 1,
		UFlags: nifTip,
	}
	copyTip(&nid.SzTip, tip)
	procShellNotify.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
}

// showMenu 右键菜单（每次打开实时重建，状态标签即实时）。
func (t *tray) showMenu() {
	if t.cb.Status == nil {
		return
	}
	running, healthy := t.cb.Status()

	menu, _, _ := procCreatePopup.Call()
	defer procDestroyMenu.Call(menu)

	statusLabel := "服务状态: 已停止"
	if running {
		if healthy {
			statusLabel = "服务状态: 运行中（健康）"
		} else {
			statusLabel = "服务状态: 运行中（健康检查未通过）"
		}
	}
	appendItem(menu, statusLabel, 0, mfGrayed)
	appendSep(menu)
	appendItem(menu, "打开 ResearchOS", uintptr(CmdOpenUI), mfString)
	appendSep(menu)
	if running {
		appendItem(menu, "启动后台服务", uintptr(CmdStart), mfGrayed)
		appendItem(menu, "停止后台服务", uintptr(CmdStop), mfString)
		appendItem(menu, "重启后台服务", uintptr(CmdRestart), mfString)
	} else {
		appendItem(menu, "启动后台服务", uintptr(CmdStart), mfString)
		appendItem(menu, "停止后台服务", uintptr(CmdStop), mfGrayed)
		appendItem(menu, "重启后台服务", uintptr(CmdRestart), mfGrayed)
	}
	appendItem(menu, "查看服务状态", uintptr(CmdStatus), mfString)
	appendSep(menu)
	appendItem(menu, "打开日志目录", uintptr(CmdOpenLogs), mfString)
	appendItem(menu, "打开数据目录", uintptr(CmdOpenData), mfString)
	appendSep(menu)
	appendItem(menu, "设置", uintptr(CmdSettings), mfString)
	appendSep(menu)
	appendItem(menu, "退出…", uintptr(CmdExit), mfString)

	// 菜单需要前台窗口否则点击外部不消失
	procSetForeground.Call(uintptr(t.hwnd))
	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	cmd, _, _ := procTrackPopup.Call(menu, uintptr(trackPopupMenu|trackReturnCMD|tpmAlignBottom),
		uintptr(pt.X), uintptr(pt.Y), 0, uintptr(t.hwnd), 0)
	if cmd != 0 {
		t.dispatch(TrayCommand(cmd))
	}
}

func appendItem(menu uintptr, label string, id uintptr, flags uintptr) {
	lp, _ := windows.UTF16PtrFromString(label)
	procAppendMenu.Call(menu, flags|mfString, id, uintptr(unsafe.Pointer(lp)))
}

func appendSep(menu uintptr) {
	procAppendMenu.Call(menu, mfSeparator, 0, 0)
}

// QuitTray 结束消息循环并移除图标。
func (t *tray) QuitTray() {
	t.mu.Lock()
	t.stopped = true
	t.mu.Unlock()
	t.removeIcon()
	procPostMessage.Call(uintptr(t.hwnd), wmClose, 0, 0)
	user32.NewProc("PostQuitMessage").Call(0)
}

// copyTip 安全拷贝托盘悬浮提示（SzTip 固定 128 个 UTF-16 单元，必须 NUL 结尾）。
func copyTip(dst *[128]uint16, tip string) {
	u16 := windows.StringToUTF16(tip)
	n := len(u16)
	if n > 127 {
		n = 127
	}
	copy(dst[:], u16[:n])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
