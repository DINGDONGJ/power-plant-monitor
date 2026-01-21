package impact

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v3/process"
)

// FileConflict 文件占用冲突信息
type FileConflict struct {
	PID  int32
	Name string
	Path string // 冲突的文件路径
}

// OpenFileInfo 进程打开的文件信息
type OpenFileInfo struct {
	PID      int32
	Name     string
	FilePath string
}

// FileChecker 文件占用检测器（跨平台，使用 gopsutil）
type FileChecker struct {
	mu sync.RWMutex
	// 缓存：文件路径 -> 打开该文件的进程列表
	fileToProcs     map[string][]OpenFileInfo
	lastRefreshTime int64 // Unix timestamp
}

// NewFileChecker 创建文件检测器
func NewFileChecker() *FileChecker {
	return &FileChecker{
		fileToProcs: make(map[string][]OpenFileInfo),
	}
}

// RefreshOpenFiles 刷新所有进程的打开文件缓存
// 这个操作较重，应该低频调用（如每 30-60 秒）
func (c *FileChecker) RefreshOpenFiles(excludePIDs map[int32]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 重建缓存
	c.fileToProcs = make(map[string][]OpenFileInfo)

	// 获取所有进程
	procs, err := process.Processes()
	if err != nil {
		return
	}

	for _, proc := range procs {
		pid := proc.Pid
		// 跳过 PID 0 和 系统进程
		if pid == 0 {
			continue
		}

		// 获取进程打开的文件
		files, err := proc.OpenFiles()
		if err != nil {
			continue
		}

		// 获取进程名
		procName, _ := proc.Name()
		if procName == "" {
			procName = "unknown"
		}

		for _, f := range files {
			// 规范化路径
			filePath := normalizePath(f.Path)
			if filePath == "" {
				continue
			}

			// 过滤掉非普通文件（socket、pipe、设备等）
			if shouldSkipFile(filePath) {
				continue
			}

			info := OpenFileInfo{
				PID:      pid,
				Name:     procName,
				FilePath: filePath,
			}
			c.fileToProcs[filePath] = append(c.fileToProcs[filePath], info)
		}
	}
}

// GetFilesOpenedByPID 获取指定进程打开的所有文件
func (c *FileChecker) GetFilesOpenedByPID(pid int32) []string {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil
	}

	files, err := proc.OpenFiles()
	if err != nil {
		return nil
	}

	var result []string
	for _, f := range files {
		filePath := normalizePath(f.Path)
		if filePath == "" || shouldSkipFile(filePath) {
			continue
		}
		result = append(result, filePath)
	}
	return result
}

// FindConflicts 查找与目标进程文件冲突的其他进程
// targetPID: 目标进程 PID
// targetFiles: 目标进程打开的文件列表（自动发现 + 配置的 WatchFiles）
// excludePIDs: 排除的 PID（其他监控目标）
func (c *FileChecker) FindConflicts(targetPID int32, targetFiles []string, excludePIDs map[int32]bool) []FileConflict {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var conflicts []FileConflict
	seen := make(map[int32]map[string]bool) // PID -> 已报告的文件

	for _, filePath := range targetFiles {
		procs, ok := c.fileToProcs[filePath]
		if !ok {
			continue
		}

		for _, proc := range procs {
			// 排除目标自身
			if proc.PID == targetPID {
				continue
			}
			// 排除其他监控目标
			if excludePIDs[proc.PID] {
				continue
			}
			// 避免同一进程同一文件重复报告
			if seen[proc.PID] == nil {
				seen[proc.PID] = make(map[string]bool)
			}
			if seen[proc.PID][filePath] {
				continue
			}
			seen[proc.PID][filePath] = true

			conflicts = append(conflicts, FileConflict{
				PID:  proc.PID,
				Name: proc.Name,
				Path: filePath,
			})
		}
	}

	return conflicts
}

// CheckFile 检查指定文件是否被其他进程占用（兼容旧接口）
func (c *FileChecker) CheckFile(filePath string, excludePID int32) []FileConflict {
	c.mu.RLock()
	defer c.mu.RUnlock()

	filePath = normalizePath(filePath)
	procs, ok := c.fileToProcs[filePath]
	if !ok {
		return nil
	}

	var conflicts []FileConflict
	for _, proc := range procs {
		if proc.PID == excludePID {
			continue
		}
		conflicts = append(conflicts, FileConflict{
			PID:  proc.PID,
			Name: proc.Name,
			Path: filePath,
		})
	}
	return conflicts
}

// CheckFiles 批量检查多个文件（兼容旧接口）
func (c *FileChecker) CheckFiles(files []string, excludePID int32) map[string][]FileConflict {
	result := make(map[string][]FileConflict)
	for _, file := range files {
		result[file] = c.CheckFile(file, excludePID)
	}
	return result
}

// normalizePath 规范化文件路径
func normalizePath(path string) string {
	if path == "" {
		return ""
	}
	// 转换为绝对路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	// 统一使用 / 作为分隔符（跨平台）
	return filepath.ToSlash(absPath)
}

// shouldSkipFile 判断是否应该跳过该文件
func shouldSkipFile(path string) bool {
	// 跳过空路径
	if path == "" {
		return true
	}

	// Linux: 跳过特殊文件系统
	if strings.HasPrefix(path, "/proc/") ||
		strings.HasPrefix(path, "/sys/") ||
		strings.HasPrefix(path, "/dev/") ||
		strings.Contains(path, "socket:") ||
		strings.Contains(path, "pipe:") ||
		strings.Contains(path, "anon_inode:") {
		return true
	}

	// Windows: 跳过系统路径
	pathLower := strings.ToLower(path)
	if strings.Contains(pathLower, "\\device\\") ||
		strings.Contains(pathLower, "\\windows\\system32\\") ||
		strings.Contains(pathLower, "\\windows\\syswow64\\") {
		return true
	}

	return false
}
