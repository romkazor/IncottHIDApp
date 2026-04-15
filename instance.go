package main

import (
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	user32           = syscall.NewLazyDLL("user32.dll")
	procCreateMutexW = kernel32.NewProc("CreateMutexW")
	procMessageBoxW  = user32.NewProc("MessageBoxW")
)

const (
	errAlreadyExists  = syscall.Errno(183)
	mbIconInformation = 0x00000040
	instanceMutexName = `Global\IncottDriver-{b5a1c2f8-4d7e-4a8f-9c1d-2e3f4a5b6c7d}`
)

var instanceMutexHandle uintptr

func acquireSingleInstance() bool {
	namePtr, err := syscall.UTF16PtrFromString(instanceMutexName)
	if err != nil {
		return true
	}
	r0, _, e1 := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(namePtr)))
	if r0 == 0 {
		return true
	}
	if e1 == errAlreadyExists {
		return false
	}
	instanceMutexHandle = r0
	return true
}

func showAlreadyRunningDialog() {
	title, _ := syscall.UTF16PtrFromString("IncottDriver")
	text, _ := syscall.UTF16PtrFromString("IncottDriver is already running.")
	procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(text)),
		uintptr(unsafe.Pointer(title)),
		mbIconInformation,
	)
}
