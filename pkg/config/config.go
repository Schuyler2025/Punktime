package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ============================================================================
// 配置目录获取函数组
// 以下三个函数按照优先级顺序获取配置目录，用于多级回退机制
// ============================================================================

/**
 * @description: 获取跨平台的标准配置目录
 * @return {string} 标准配置目录路径
 * @return {error} 错误信息
 */
func getStandardConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	var configDir string
	switch runtime.GOOS {
	case "windows":
		configDir = filepath.Join(homeDir, "AppData", "Local", "Punktime")
	case "linux":
		configDir = filepath.Join(homeDir, ".config", "Punktime")
	case "darwin":
		configDir = filepath.Join(homeDir, "Library", "Application Support", "Punktime")
	default:
		configDir = filepath.Join(homeDir, ".Punktime")
	}

	return createAndVerifyDir(configDir)
}

/**
 * @description: 获取备用用户配置目录（第二优先级）
 * @return {string} 备用配置目录路径
 * @return {error} 错误信息
 */
func getFallbackUserDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(homeDir, ".Punktime_Config")
	return createAndVerifyDir(configDir)
}

/**
 * @description: 获取项目根配置目录（第三优先级）
 * @return {string} 项目配置目录路径
 * @return {error} 错误信息
 */
func getProjectRootDir() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}
	configDir := filepath.Join(currentDir, "config")
	return createAndVerifyDir(configDir)
}

func formatAttempts(attempts []string) string {
	result := ""
	for _, attempt := range attempts {
		result += attempt + "\n"
	}
	return result
}

/**
 * @description: 按照优先级顺序获取可用的配置目录
 * 函数会依次尝试以下配置目录，直到找到第一个可用的目录：
 * 1. 标准配置目录（跨平台标准位置）
 * 2. 备用用户目录（用户主目录下的备用位置）
 * 3. 项目根目录（当前工作目录下的配置目录）
 *
 * @return {string} 成功找到的配置目录路径，如果所有目录都不可用则返回空字符串
 * @return {[]string} 尝试过程的详细记录，包含每个目录的尝试结果和错误信息
 *
 * @example
 * // 获取配置目录
 * configDir, attempts := getConfigDirWithPriority()
 * if configDir == "" {
 *     log.Printf("所有配置目录都不可用，尝试记录：%v", attempts)
 *     return
 * }
 * fmt.Printf("使用配置目录: %s\n", configDir)
 */
func getConfigDirWithPriority() (string, []string) {
	attempts := []string{}

	if dir, err := getStandardConfigDir(); err == nil {
		attempts = append(attempts, fmt.Sprintf("Standard Config Dir: %s", dir))
		return dir, attempts
	} else {
		attempts = append(attempts, fmt.Sprintf("Fallback User Config Dir: %v", err))
	}

	if dir, err := getFallbackUserDir(); err == nil {
		attempts = append(attempts, fmt.Sprintf("Fallback User Config Dir: %s", dir))
		return dir, attempts
	} else {
		attempts = append(attempts, fmt.Sprintf("Project Root Config Dir: %v", err))
	}

	if dir, err := getProjectRootDir(); err == nil {
		attempts = append(attempts, fmt.Sprintf("Project Root Config Dir: %s", dir))
		return dir, attempts
	} else {
		attempts = append(attempts, fmt.Sprintf("Project Root Config Dir: %v", err))
	}

	return "", attempts
}

func GetConfigPath() (string, string, error) {
	configDir, attempts := getConfigDirWithPriority()
	if configDir == "" {
		return "", "", fmt.Errorf("failed to get config directory: %s", formatAttempts(attempts))
	}
	return filepath.Join(configDir, "Punktime.config.json"), configDir, nil
}

func createAndVerifyDir(dir string) (string, error) {
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create config directory %s: %w", dir, err)
	}

	if err := verifyDirWritable(dir); err != nil {
		return "", fmt.Errorf("failed to verify directory writable: %w", err)
	}
	return dir, nil
}

/**
 * @description: 验证目录的可写性和可读性
 * 函数会执行以下验证步骤：
 * 1. 检查目录是否存在且为目录类型
 * 2. 尝试在目录中创建临时测试文件验证写权限
 * 3. 删除临时测试文件
 * 4. 尝试读取目录内容验证读权限
 *
 * @param {string} dir 需要验证的目录路径
 * @return {error} 错误信息，包括目录不存在、不是目录、无写权限或无读权限
 *
 * @example
 * // 验证目录可写性
 * err := verifyDirWritable("/path/to/config")
 * if err != nil {
 *     log.Printf("目录不可用: %v", err)
 *     return
 * }
 * fmt.Println("目录可正常读写")
 */
func verifyDirWritable(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("failed to stat directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}

	testFile := filepath.Join(dir, ".write_test")
	file, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("directory %s can not write %s: %w", dir, testFile, err)
	}
	file.Close()

	os.Remove(testFile)

	_, err = os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("directory %s can not read: %w", dir, err)
	}
	return nil
}
