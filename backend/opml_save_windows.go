//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	comdlg32         = syscall.NewLazyDLL("comdlg32.dll")
	getSaveFileNameW = comdlg32.NewProc("GetSaveFileNameW")
)

type openFilenameW struct {
	size             uint32
	owner            uintptr
	instance         uintptr
	filter           *uint16
	customFilter     *uint16
	maxCustomFilter  uint32
	filterIndex      uint32
	file             *uint16
	maxFile          uint32
	fileTitle        *uint16
	maxFileTitle     uint32
	initialDirectory *uint16
	title            *uint16
	flags            uint32
	fileOffset       uint16
	fileExtension    uint16
	defaultExtension *uint16
	customData       uintptr
	hook             uintptr
	templateName     *uint16
	reserved         uintptr
	reservedValue    uint32
	flagsEx          uint32
}

func chooseOPMLExportPath(filename string) (string, bool, error) {
	if filename == "" {
		filename = "rss-subscriptions.opml"
	}
	file, err := syscall.UTF16FromString(filename)
	if err != nil {
		return "", false, fmt.Errorf("invalid default filename: %w", err)
	}
	filter, _ := syscall.UTF16FromString("OPML files (*.opml)\x00*.opml\x00All files (*.*)\x00*.*\x00\x00")
	ext, _ := syscall.UTF16PtrFromString("opml")
	title, _ := syscall.UTF16PtrFromString("导出订阅")
	buffer := make([]uint16, 32768)
	copy(buffer, file)
	config := openFilenameW{
		size:             uint32(unsafe.Sizeof(openFilenameW{})),
		filter:           &filter[0],
		filterIndex:      1,
		file:             &buffer[0],
		maxFile:          uint32(len(buffer)),
		title:            title,
		flags:            0x00080000 | 0x00000800 | 0x00000002 | 0x00000008,
		defaultExtension: ext,
	}
	ok, _, callErr := getSaveFileNameW.Call(uintptr(unsafe.Pointer(&config)))
	if ok == 0 {
		if code, _, _ := syscall.SyscallN(comdlg32.NewProc("CommDlgExtendedError").Addr()); code != 0 {
			return "", false, fmt.Errorf("save dialog failed (code %d): %v", code, callErr)
		}
		return "", false, nil
	}
	return syscall.UTF16ToString(buffer), true, nil
}
