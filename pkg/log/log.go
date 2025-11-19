/*
 * @Author: Schuyler schuylerhu@gmail.com
 * @Date: 2025-11-05 08:59:32
 * @LastEditors: Schuyler schuylerhu@gmail.com
 * @LastEditTime: 2025-11-19 12:41:39
 * @FilePath: \Punktime\pkg\log\log.go
 * @Description: Punktime应用程序的日志系统模块
 * 功能特性：
 * - 多级别日志记录（DEBUG/INFO/WARNING/ERROR/FATAL）
 * - 自动日志文件轮转和大小限制（10MB）
 * - 系统启动时记录详细的系统信息（操作系统版本、CPU架构、内存使用情况等）
 * - 跨平台支持（Windows/Linux/macOS）
 * - 管理员权限检测
 * - 线程安全的日志写入
 * - 自动清理旧日志文件（保留最近5个）
 *
 * 主要函数：
 * - InitializeLogSystem(): 初始化日志系统
 * - WriteLog(): 核心日志写入函数
 * - rotateLogFile(): 日志文件轮转
 * - 各种LOG_*宏：便捷的日志记录接口
 *
 * 系统信息收集：
 * - 操作系统版本检测
 * - 内存使用情况监控
 * - CPU架构和核心数
 * - 管理员权限状态
 *
 * Copyright (c) 2025 by Schuyler, All Rights Reserved.
 */
package log

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	"github.com/Schuyler2025/Punktime/pkg/config"
)

type LogLevel int

// 日志级别常量
const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarning
	LogLevelError
	LogLevelFatal
)

// 日志文件配置
const (
	maxLogSize  = 10 * 1024 * 1024
	maxLogFiles = 5
	// 日志轮转检查间隔
	logRollInterval = 1000
)

// 日志系统状态变量
var (
	minLogLevel = LogLevelDebug
	logFile     *os.File
	mu          sync.Mutex
	// 日志写入计数器和当前文件大小
	writeCount  uint64
	currentSize int64
	// 上次日志轮转检查时间
	lastRollCheck uint64
)

/**
 * @description: 初始化日志系统
 * 功能包括：
 * - 创建或打开日志文件（位于用户配置目录）
 * - 初始化日志系统的内部状态变量
 * - 记录详细的系统启动信息（操作系统版本、CPU架构、内存使用情况等）
 * - 设置日志轮转和清理机制
 *
 * @return {bool} true - 日志系统初始化成功，可以正常记录日志
 * @return {bool} false - 初始化失败，日志记录功能不可用
 *
 * @example
 * // 在应用程序启动时调用
 * if !log.InitializeLogSystem() {
 *     fmt.Println("日志系统初始化失败")
 *     return
 * }
 */
func InitializeLogSystem() bool {
	logFilePath, err := getLogFilePath()
	if err != nil {
		return false
	}

	logFile, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return false
	}

	atomic.StoreUint64(&writeCount, 0)
	atomic.StoreInt64(&currentSize, 0)
	atomic.StoreUint64(&lastRollCheck, 0)

	LOG_INFO("=========================================================")
	LOG_INFO("--------------------System Infomation--------------------")

	logOSVersion()
	logCPUArch()
	logMemoryInfo()
	logAdminPrivilege()

	LOG_INFO("--------------------Application Start--------------------")
	LOG_INFO("Log system initialized successfully")

	return true
}

func logAdminPrivilege() {
	isAdmin := checkAdminPrivilege()
	if isAdmin {
		LOG_INFO("Admin Privilege: Yes")
	} else {
		LOG_INFO("Admin Privilege: No")
	}
}

func checkAdminPrivilege() bool {
	if runtime.GOOS == "windows" {
		// Windows系统检查管理员权限
		var sid *windows.SID
		err := windows.AllocateAndInitializeSid(
			&windows.SECURITY_NT_AUTHORITY,
			2,
			windows.SECURITY_BUILTIN_DOMAIN_RID,
			windows.DOMAIN_ALIAS_RID_ADMINS,
			0, 0, 0, 0, 0, 0,
			&sid)
		if err != nil {
			return false
		}
		defer windows.FreeSid(sid)

		token := windows.Token(0)
		member, err := token.IsMember(sid)
		if err != nil {
			return false
		}
		return member
	} else {
		// Unix-like系统检查root权限
		return os.Geteuid() == 0
	}
}

// ============================================================================
// 系统信息记录函数组
// 以下函数用于在应用程序启动时记录详细的系统环境信息
// ============================================================================

/**
 * @description: 记录内存使用信息
 * 记录Go运行时内存统计信息，包括：
 * - 已分配内存和系统内存
 * - 堆内存和栈内存使用情况
 * - Windows系统下额外记录系统物理内存信息
 *
 * @example
 * // 记录内存信息
 * logMemoryInfo()
 */
func logMemoryInfo() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	LOG_INFO("Memory Alloc: %.2f MB, Sys: %.2f MB", float64(memStats.Alloc)/1024/1024, float64(memStats.Sys)/1024/1024)
	LOG_INFO("Memory Heap: %.2f MB, Stack: %.2f MB", float64(memStats.HeapAlloc)/1024/1024, float64(memStats.StackInuse)/1024/1024)

	if runtime.GOOS == "windows" {
		logWindowsMemoryInfo()
	}
}

/**
 * @description: 记录Windows系统内存信息
 * 使用Windows API获取系统物理内存信息，包括：
 * - 总物理内存和可用物理内存
 * - 内存负载百分比
 * - 页面文件信息
 *
 * @example
 * // 记录Windows内存信息
 * logWindowsMemoryInfo()
 */
func logWindowsMemoryInfo() {
	type memoryStatusEx struct {
		Length               uint32
		MemoryLoad           uint32
		TotalPhys            uint64
		AvailPhys            uint64
		TotalPageFile        uint64
		AvailPageFile        uint64
		TotalVirtual         uint64
		AvailVirtual         uint64
		AvailExtendedVirtual uint64
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	globalMemoryStatusEx := kernel32.NewProc("GlobalMemoryStatusEx")

	var memInfo memoryStatusEx
	memInfo.Length = uint32(unsafe.Sizeof(memInfo))

	ret, _, err := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memInfo)))
	if ret != 0 {
		LOG_INFO("System Memory - Total: %.2f GB, Available: %.2f GB",
			float64(memInfo.TotalPhys)/1024/1024/1024,
			float64(memInfo.AvailPhys)/1024/1024/1024)
		LOG_INFO("Memory Load: %d%%", memInfo.MemoryLoad)
	} else if err != nil && err != syscall.Errno(0) {
		LOG_INFO("Memory Info: Unable to retrieve (error: %v)", err)
	}
}

/**
 * @description: 记录CPU架构和核心信息
 * 记录系统CPU相关信息，包括：
 * - CPU架构类型（GOARCH）
 * - CPU核心数量
 * - GOMAXPROCS设置
 *
 * @example
 * // 记录CPU信息
 * logCPUArch()
 */
func logCPUArch() {
	LOG_INFO("CPU Arch: %s", runtime.GOARCH)
	LOG_INFO("OS Arch: %d", runtime.NumCPU())
	LOG_INFO("GOMAXPROCS: %d", runtime.GOMAXPROCS(0))
}

/**
 * @description: 记录操作系统版本信息
 * 根据当前操作系统类型调用对应的版本记录函数：
 * - Windows: 记录Windows版本
 * - Linux: 记录Linux发行版和内核版本
 * - macOS: 记录macOS版本
 * - 其他: 记录警告信息
 *
 * @example
 * // 记录操作系统信息
 * logOSVersion()
 */
func logOSVersion() {
	switch runtime.GOOS {
	case "windows":
		logWindowsVersion()
	case "linux":
		logLinuxVersion()
	case "darwin":
		logMacVersion()
	default:
		LOG_WARNING("Unsupported OS: %s", runtime.GOOS)
	}
}

/**
 * @description: 记录Windows操作系统版本
 * 使用cmd ver命令获取Windows版本信息
 * 处理中文编码问题，确保正确显示中文版本信息
 *
 * @example
 * // 记录Windows版本
 * logWindowsVersion()
 */
func logWindowsVersion() {
	cmd := exec.Command("cmd", "/c", "ver")
	output, err := cmd.Output()

	if err != nil {
		LOG_WARNING("Windows Version: Unable to retrieve (error: %v)", err)
		return
	}

	decoder := simplifiedchinese.GBK.NewDecoder()
	utf8Output, _, err := transform.Bytes(decoder, output)
	if err != nil {
		LOG_WARNING("Windows Version: Encoding error (error: %v)", err)
		return
	}

	versionStr := strings.TrimSpace(string(utf8Output))
	LOG_INFO("Windows Version: %s", versionStr)
}

/**
 * @description: 记录Linux操作系统版本
 * 读取/etc/os-release文件获取Linux发行版信息
 * 使用uname命令获取内核版本信息
 *
 * @example
 * // 记录Linux版本
 * logLinuxVersion()
 */
func logLinuxVersion() {
	content, err := os.ReadFile("/etc/os-release")
	if err == nil {
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				prettyName := strings.TrimPrefix(line, "PRETTY_NAME=")
				prettyName = strings.Trim(prettyName, "\"")
				LOG_INFO("Linux Distribution: %s", prettyName)
				break
			}
		}
	}

	cmd := exec.Command("uname", "-r")
	output, err := cmd.Output()
	if err == nil {
		LOG_INFO("Linux Kernel Version: %s", strings.TrimSpace(string(output)))
	}

}

/**
 * @description: 记录macOS操作系统版本
 * 使用sw_vers命令获取macOS产品版本信息
 *
 * @example
 * // 记录macOS版本
 * logMacVersion()
 */
func logMacVersion() {
	cmd := exec.Command("sw_vers", "-productVersion")
	output, err := cmd.Output()
	if err == nil {
		LOG_INFO("Mac Version: %s", strings.TrimSpace(string(output)))
	}
}

func getLogFilePath() (string, error) {
	_, configDir, err := config.GetConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "Punktime.log.json"), nil
}

// ============================================================================
// 日志文件处理函数组
// 这些函数负责日志的写入、轮转、清理等文件操作
// ============================================================================

/**
 * @description: 写入日志到文件的主函数
 * 函数执行以下操作：
 * 1. 检查日志文件是否已初始化
 * 2. 检查日志级别是否满足最小级别要求
 * 3. 使用互斥锁确保线程安全
 * 4. 生成带时间戳和级别的格式化日志条目
 * 5. 写入文件并更新文件大小统计
 * 6. 定期检查是否需要轮转日志文件
 *
 * @param {LogLevel} level - 日志级别
 * @param {string} format - 格式化字符串
 * @param {...interface{}} args - 格式化参数
 *
 * @example
 * WriteLog(LogLevelInfo, "User %s logged in", "john")
 * // 输出: [2024-01-15 10:30:25] [INFO] User john logged in

 * @note 日志轮转检查基于写入次数而非实时时间
 */
func WriteLog(level LogLevel, format string, args ...interface{}) {
	if logFile == nil {
		return
	}

	if level < minLogLevel {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	currentCount := atomic.AddUint64(&writeCount, 1)

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	levelStr := getLevelString(level)
	logEntry := fmt.Sprintf("[%s] [%s] %s\n", timestamp, levelStr, fmt.Sprintf(format, args...))

	if _, err := logFile.WriteString(logEntry); err != nil {
		return
	}

	newSize := atomic.AddInt64(&currentSize, int64(len(logEntry)))

	if currentCount-atomic.LoadUint64(&lastRollCheck) >= logRollInterval {
		atomic.StoreUint64(&lastRollCheck, currentCount)
		checkAndRotateLog(newSize)
	}

}

/**
 * @description: 检查并轮转日志文件
 * 当当前日志文件大小超过最大限制时，触发日志轮转
 *
 * @param {int64} currentSize - 当前日志文件大小
 */
func checkAndRotateLog(currentSize int64) {

	if currentSize < maxLogSize {
		return
	}

	if err := rotateLogFile(); err != nil {
		LOG_ERROR("Failed to rotate log file: %v", err)
		return
	}

	atomic.StoreInt64(&currentSize, 0)
	LOG_INFO("Log file rotated successfully, new file created")
}

/**
 * @description: 执行日志文件轮转操作
 * 轮转流程：
 * 1. 关闭当前日志文件
 * 2. 重命名当前文件为备份文件（添加时间戳）
 * 3. 创建新的日志文件
 * 4. 清理过期的旧日志文件
 *
 * @return {error} 轮转过程中出现的错误
 *
 * @example
 * if err := rotateLogFile(); err != nil {
 *     LOG_ERROR("Log rotation failed: %v", err)
 * }
 */
func rotateLogFile() error {
	if logFile == nil {
		return fmt.Errorf("log file is not initialized")
	}

	if err := logFile.Close(); err != nil {
		return fmt.Errorf("failed to close log file: %w", err)
	}

	currentPath, err := getLogFilePath()
	if err != nil {
		return fmt.Errorf("failed to get log file path: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	backupPath := fmt.Sprintf("%s.%s", currentPath, timestamp)
	if err := os.Rename(currentPath, backupPath); err != nil {
		return fmt.Errorf("failed to rename log file: %w", err)
	}

	newFile, err := os.Create(currentPath)
	if err != nil {
		return fmt.Errorf("failed to create new log file: %w", err)
	}
	logFile = newFile

	cleanupOldLogs(currentPath)

	return nil
}

/**
 * @description: 清理过期的旧日志文件，防止磁盘空间被无限占用
 * 函数执行以下清理流程：
 * 1. 解析基础路径获取目录和基础文件名
 * 2. 读取目录中的所有文件
 * 3. 筛选出符合条件的日志备份文件
 * 4. 按修改时间对文件进行排序（从旧到新）
 * 5. 保留最近的一定数量文件，删除更早的文件
 *
 * @param {string} basePath - 当前日志文件的基础路径（如：/path/to/app.log）
 *
 * @example
 * // 假设maxLogFiles为5，当前有8个备份文件
 * cleanupOldLogs("/var/log/app.log")
 * // 将保留最新的5个文件，删除最旧的3个文件
 *
 * @see isLogBackupFile 判断文件是否为日志备份文件
 * @see sortLogFilesByTime 按时间排序文件列表
 *
 * @note 备份文件命名格式：基础文件名.时间戳（如：app.log.20240115103025）
 */
func cleanupOldLogs(basePath string) {
	dir := filepath.Dir(basePath)
	baseName := filepath.Base(basePath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	var logFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && isLogBackupFile(entry.Name(), baseName) {
			logFiles = append(logFiles, entry.Name())
		}
	}

	sortLogFilesByTime(logFiles)

	if len(logFiles) > maxLogFiles {
		for i := maxLogFiles; i < len(logFiles); i++ {
			if err := os.Remove(filepath.Join(dir, logFiles[i])); err != nil {
				LOG_ERROR("Failed to remove old log file %s: %v", logFiles[i], err)
			}
		}
	}

}

func sortLogFilesByTime(files []string) {
	for i := 0; i < len(files)-1; i++ {
		for j := i + 1; j < len(files); j++ {
			infoI, errI := os.Stat(files[i])
			infoJ, errJ := os.Stat(files[j])

			if errI != nil || errJ != nil {
				continue
			}
			if infoI.ModTime().Before(infoJ.ModTime()) {
				files[i], files[j] = files[j], files[i]
			}
		}
	}
}

func isLogBackupFile(filename, baseName string) bool {
	return len(filename) > len(baseName) && filename[:len(baseName)] == baseName
}

func getLevelString(level LogLevel) string {
	switch level {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarning:
		return "WARNING"
	case LogLevelError:
		return "ERROR"
	case LogLevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

func LOG_DEBUG(format string, args ...interface{}) {
	WriteLog(LogLevelDebug, format, args...)
}

func LOG_INFO(format string, args ...interface{}) {
	WriteLog(LogLevelInfo, format, args...)
}

func LOG_WARNING(format string, args ...interface{}) {
	WriteLog(LogLevelWarning, format, args...)
}

func LOG_ERROR(format string, args ...interface{}) {
	WriteLog(LogLevelError, format, args...)
}

func LOG_FATAL(format string, args ...interface{}) {
	WriteLog(LogLevelFatal, format, args...)
}

var (
	inCrashHandler int32
)

func SetupExceptionHandler() {

	sigChan := make(chan os.Signal, 1)

	signals := []os.Signal{
		syscall.SIGFPE,
		syscall.SIGILL,
		syscall.SIGSEGV,
		syscall.SIGTERM,
		syscall.SIGABRT,
		syscall.SIGINT,
	}

	signal.Notify(sigChan, signals...)

	go signalHandler(sigChan)

}

func signalHandler(sigChan chan os.Signal) {
	for {
		sig := <-sigChan
		handleSignal(sig)
	}
}

/*
*

  - @description: 处理接收到的系统信号，执行紧急日志记录和程序退出

  - 函数通过原子操作确保同一时间只有一个信号被处理，避免重复处理导致的竞态条件。

  - 处理流程：

  - 1. 检查是否已有信号正在处理（使用原子操作防止重复进入）

  - 2. 如果已有处理程序运行，直接退出程序

  - 3. 获取信号的描述信息和编号

  - 4. 执行紧急日志记录，将信号信息写入日志文件

  - 5. 使用信号对应的编号退出程序

  - @example

  - // 当接收到SIGSEGV信号时

  - handleSignal(syscall.SIGSEGV)

  - // 输出日志：[时间戳] [FATAL] Fatal signal occurred: SIGSEGV Segmentation Fault (signal number: 11)

  - // 程序以11退出码退出
    *

  - @see getSignalDescription 获取信号描述信息

  - @see getSignalNumber 获取信号编号

  - @see emergencyLogWrite 执行紧急日志记录
*/
func handleSignal(sig os.Signal) {
	if !atomic.CompareAndSwapInt32(&inCrashHandler, 0, 1) {
		os.Exit(getSignalNumber(sig))
		return
	}

	signalDesc := getSignalDescription(sig)
	signalNum := getSignalNumber(sig)

	emergencyLogWrite(signalDesc, signalNum)

	os.Exit(signalNum)

}

/**
 * @description: 紧急日志写入函数，在程序异常退出前记录关键信息
 * 函数在程序接收到致命信号时被调用，用于记录异常发生的时间、信号描述和信号编号。
 * 执行流程：
 * 1. 检查日志文件是否已打开（logFile != nil）
 * 2. 生成包含时间戳的格式化日志消息
 * 3. 将消息写入日志文件
 * 4. 强制同步到磁盘确保数据不丢失
 * 5. 关闭日志文件并置空指针
 *
 * 这个函数是异常处理流程的最后一步，确保即使程序崩溃也能留下调试信息。
 *
 * @example
 * // 当发生段错误时
 * emergencyLogWrite("SIGSEGV Segmentation Fault", 11)
 * // 日志输出：[2024-01-15 10:30:25] [FATAL] Fatal signal occurred: SIGSEGV Segmentation Fault (signal number: 11)
 *
 * @note 此函数执行后程序将立即退出，确保日志写入完成后再退出
 */
func emergencyLogWrite(signalDesc string, signalNum int) {
	if logFile != nil {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		message := fmt.Sprintf("[%s] [FATAL] Fatal signal occurred: %s (signal number: %d)\n", timestamp, signalDesc, signalNum)

		logFile.WriteString(message)
		logFile.Sync()
		logFile.Close()
		logFile = nil
	}
}

func getSignalDescription(sig os.Signal) string {
	switch sig {
	case syscall.SIGFPE:
		return "SIGFPE Floating Point Exception"
	case syscall.SIGILL:
		return "SIGILL Illegal Instruction"
	case syscall.SIGSEGV:
		return "SIGSEGV Segmentation Fault"
	case syscall.SIGTERM:
		return "SIGTERM Termination Signal"
	case syscall.SIGABRT:
		return "SIGABRT Abnormal Termination Signal"
	case syscall.SIGINT:
		return "SIGINT Interrupt Signal"
	default:
		return "Unknown Signal"
	}
}

func getSignalNumber(sig os.Signal) int {
	switch sig {
	case syscall.SIGFPE:
		return int(syscall.SIGFPE)
	case syscall.SIGILL:
		return int(syscall.SIGILL)
	case syscall.SIGSEGV:
		return int(syscall.SIGSEGV)
	case syscall.SIGTERM:
		return int(syscall.SIGTERM)
	case syscall.SIGABRT:
		return int(syscall.SIGABRT)
	case syscall.SIGINT:
		return int(syscall.SIGINT)
	default:
		return 1
	}
}
