//go:build linux

package provider

func New() ProcProvider {
	return newCommonProvider(
		// matchProcessName: Linux 直接匹配
		func(procName, targetName string) bool {
			return procName == targetName
		},
		// formatCmdline: Linux 直接返回
		func(exe string) string {
			return exe
		},
		// getHandleCount: Linux 使用 gopsutil 的 NumFDs (返回 nil 使用默认实现)
		nil,
		// getPriority: Linux 使用 gopsutil 的 Nice (返回 nil 使用默认实现)
		nil,
	)
}
