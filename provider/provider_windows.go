//go:build windows

package provider

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modkernel32                 = syscall.NewLazyDLL("kernel32.dll")
	modpsapi                    = syscall.NewLazyDLL("psapi.dll")
	modversion                  = syscall.NewLazyDLL("version.dll")
	procGetProcessHandleCount   = modkernel32.NewProc("GetProcessHandleCount")
	procOpenProcess             = modkernel32.NewProc("OpenProcess")
	procCloseHandle             = modkernel32.NewProc("CloseHandle")
	procGetProcessMemoryInfo    = modpsapi.NewProc("GetProcessMemoryInfo")
	procGetPriorityClass        = modkernel32.NewProc("GetPriorityClass")
	procGetFileVersionInfoW     = modversion.NewProc("GetFileVersionInfoW")
	procGetFileVersionInfoSizeW = modversion.NewProc("GetFileVersionInfoSizeW")
	procVerQueryValueW          = modversion.NewProc("VerQueryValueW")

	// 文件描述缓存（避免重复调用 Windows API）
	fileDescCache   = make(map[string]string)
	fileDescCacheMu sync.RWMutex
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

// getFileDescription 获取可执行文件的描述信息（带缓存）
func getFileDescription(exePath string) string {
	if exePath == "" {
		return ""
	}

	// 先检查缓存
	fileDescCacheMu.RLock()
	if desc, ok := fileDescCache[exePath]; ok {
		fileDescCacheMu.RUnlock()
		return desc
	}
	fileDescCacheMu.RUnlock()

	// 缓存未命中，调用 Windows API
	desc := getFileDescriptionFromAPI(exePath)

	// 写入缓存
	fileDescCacheMu.Lock()
	fileDescCache[exePath] = desc
	fileDescCacheMu.Unlock()

	return desc
}

// getFileDescriptionFromAPI 从 Windows API 获取文件描述
func getFileDescriptionFromAPI(exePath string) string {
	pathPtr, err := windows.UTF16PtrFromString(exePath)
	if err != nil {
		return ""
	}

	// 获取版本信息大小
	size, _, _ := procGetFileVersionInfoSizeW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
	)
	if size == 0 {
		return ""
	}

	// 分配缓冲区并获取版本信息
	data := make([]byte, size)
	ret, _, _ := procGetFileVersionInfoW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		size,
		uintptr(unsafe.Pointer(&data[0])),
	)
	if ret == 0 {
		return ""
	}

	// 查询语言和代码页
	var langCodePage *uint32
	var langLen uint32
	subBlockPtr, _ := windows.UTF16PtrFromString(`\VarFileInfo\Translation`)
	ret, _, _ = procVerQueryValueW.Call(
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(subBlockPtr)),
		uintptr(unsafe.Pointer(&langCodePage)),
		uintptr(unsafe.Pointer(&langLen)),
	)

	if ret == 0 || langLen == 0 {
		// 尝试常见的语言代码页
		return tryGetDescription(data, 0x040904B0) // 英文 Unicode
	}

	// 使用第一个语言代码页
	langCP := *langCodePage
	lang := uint16(langCP & 0xFFFF)
	cp := uint16(langCP >> 16)
	fullLangCP := (uint32(lang) << 16) | uint32(cp)

	return tryGetDescription(data, fullLangCP)
}

func tryGetDescription(data []byte, langCP uint32) string {
	lang := uint16(langCP >> 16)
	cp := uint16(langCP & 0xFFFF)

	// 构建查询路径
	queryPath := fmt.Sprintf(`\StringFileInfo\%04x%04x\FileDescription`, lang, cp)
	queryPtr, _ := windows.UTF16PtrFromString(queryPath)

	var valuePtr *uint16
	var valueLen uint32
	ret, _, _ := procVerQueryValueW.Call(
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(queryPtr)),
		uintptr(unsafe.Pointer(&valuePtr)),
		uintptr(unsafe.Pointer(&valueLen)),
	)

	if ret == 0 || valueLen == 0 {
		return ""
	}

	// 转换为 Go 字符串
	return windows.UTF16PtrToString(valuePtr)
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
		// getFileDescription: Windows 使用版本信息 API 获取文件描述
		getFileDescription,
		// divideByNumCPU: Windows 风格，进程 CPU 最大 100%
		true,
	)
}
