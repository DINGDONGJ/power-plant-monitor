//go:build windows

package provider

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	modkernel32               = syscall.NewLazyDLL("kernel32.dll")
	modpsapi                  = syscall.NewLazyDLL("psapi.dll")
	procGetProcessHandleCount = modkernel32.NewProc("GetProcessHandleCount")
	procOpenProcess           = modkernel32.NewProc("OpenProcess")
	procCloseHandle           = modkernel32.NewProc("CloseHandle")
	procGetProcessMemoryInfo  = modpsapi.NewProc("GetProcessMemoryInfo")
	procGetPriorityClass      = modkernel32.NewProc("GetPriorityClass")
)

const (
	PROCESS_QUERY_INFORMATION = 0x0400
	PROCESS_VM_READ           = 0x0010
)

// PROCESS_MEMORY_COUNTERS_EX 结构体
type processMemoryCountersEx struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr // 页面缓冲池
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr // 非页面缓冲池
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
}

// getProcessHandleCount 获取进程句柄数
func getProcessHandleCount(pid int32) int32 {
	handle, _, _ := procOpenProcess.Call(
		uintptr(PROCESS_QUERY_INFORMATION),
		0,
		uintptr(pid),
	)
	if handle == 0 {
		return 0
	}
	defer procCloseHandle.Call(handle)

	var count uint32
	ret, _, _ := procGetProcessHandleCount.Call(handle, uintptr(unsafe.Pointer(&count)))
	if ret == 0 {
		return 0
	}
	return int32(count)
}

// getProcessPriority 获取进程优先级
func getProcessPriority(pid int32) int32 {
	handle, _, _ := procOpenProcess.Call(
		uintptr(PROCESS_QUERY_INFORMATION),
		0,
		uintptr(pid),
	)
	if handle == 0 {
		return 0
	}
	defer procCloseHandle.Call(handle)

	priorityClass, _, _ := procGetPriorityClass.Call(handle)
	// 将 Windows 优先级类到数值的映射
	// IDLE_PRIORITY_CLASS (0x40) = 4
	// BELOW_NORMAL_PRIORITY_CLASS (0x4000) = 6
	// NORMAL_PRIORITY_CLASS (0x20) = 8
	// ABOVE_NORMAL_PRIORITY_CLASS (0x8000) = 10
	// HIGH_PRIORITY_CLASS (0x80) = 13
	// REALTIME_PRIORITY_CLASS (0x100) = 24
	switch priorityClass {
	case 0x40:
		return 4 // Idle
	case 0x4000:
		return 6 // Below Normal
	case 0x20:
		return 8 // Normal
	case 0x8000:
		return 10 // Above Normal
	case 0x80:
		return 13 // High
	case 0x100:
		return 24 // Realtime
	default:
		return 8 // Default to Normal
	}
}

func New() ProcProvider {
	return newCommonProvider(
		// matchProcessName: Windows 需要匹配 .exe 后缀
		func(procName, targetName string) bool {
			return procName == targetName || procName == targetName+".exe"
		},
		// formatCmdline: Windows 给路径加引号
		func(exe string) string {
			return fmt.Sprintf("\"%s\"", exe)
		},
		// getHandleCount: Windows 使用 GetProcessHandleCount API
		getProcessHandleCount,
		// getPriority: Windows 使用 GetPriorityClass API
		getProcessPriority,
	)
}
