package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// dpapi.go — Windows DPAPI（当前用户作用域）封装。
// 用于 config.json 中敏感字段（数据库口令、API Token、SECRET_KEY）的静态加密。
// 不自行发明加密方案；DPAPI 密钥由 Windows 按用户管理，其它用户/裸磁盘无法解密。

const dpapiFlags = windows.CRYPTPROTECT_UI_FORBIDDEN

// dpapiProtect 加密明文，返回 base64 由调用方处理。
func dpapiProtect(plain []byte) ([]byte, error) {
	if len(plain) == 0 {
		return nil, nil
	}
	in := windows.DataBlob{Size: uint32(len(plain)), Data: &plain[0]}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, dpapiFlags, &out); err != nil {
		return nil, fmt.Errorf("CryptProtectData: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	buf := make([]byte, out.Size)
	copy(buf, unsafe.Slice(out.Data, out.Size))
	return buf, nil
}

// dpapiUnprotect 解密 DPAPI blob。
func dpapiUnprotect(blob []byte) ([]byte, error) {
	if len(blob) == 0 {
		return nil, nil
	}
	in := windows.DataBlob{Size: uint32(len(blob)), Data: &blob[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, dpapiFlags, &out); err != nil {
		return nil, fmt.Errorf("CryptUnprotectData: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	buf := make([]byte, out.Size)
	copy(buf, unsafe.Slice(out.Data, out.Size))
	return buf, nil
}
