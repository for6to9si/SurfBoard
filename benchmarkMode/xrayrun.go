package benchmarkMode

import (
	"SurfBoard/conf"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

var settings conf.BenchmarkSettings

var env []string

var xrayCmd *exec.Cmd

var lockFile string

func Init(benchmarkSettings conf.BenchmarkSettings) {

	settings = benchmarkSettings

	env = ToEnvSlice(benchmarkSettings.Env)

	lockFile = benchmarkSettings.Paths.XraylockFile

	//address = fmt.Sprintf("dns:///% Rathod s:%d", grpc.Target.IP, grpc.Target.Port)
	fmt.Printf("Используется GRPC IP: %s, Порт: %d\n", settings.Grpc.Target.IP, settings.Grpc.Target.Port)

}

// ToEnvSlice преобразует Env в срез строк формата KEY=VALUE
func ToEnvSlice(e conf.Env) []string {
	var env []string
	if e.XrayLocationAsset != "" {
		env = append(env, fmt.Sprintf("XRAY_LOCATION_ASSET=%s", e.XrayLocationAsset))
	}
	//if e.XrayLocationConfdir != "" {
	//	env = append(env, fmt.Sprintf("XRAY_LOCATION_CONFDIR=%s", e.XrayLocationConfdir))
	//}
	if e.XrayRayBufferSize != 0 {
		env = append(env, fmt.Sprintf("XRAY_RAY_BUFFER_SIZE=%d", e.XrayRayBufferSize))
	}
	return env
}

// Запускает Xray-core и возвращает строку с результатом
func StartXray() string {

	// Проверяем, запущен ли процесс
	if IsXrayRunning() {
		return "Xray-core уже запущен"
	}

	xrayCmd = exec.Command(settings.Paths.XrayExecutable, "run", "-confdir", settings.Env.XrayLocationConfdir)

	// Добавление переменных окружения к системным
	xrayCmd.Env = append(os.Environ(), env...)

	// Вывод для проверки
	if xrayCmd != nil {
		fmt.Println("Переменные окружения:", xrayCmd.Env)
	} else {
		fmt.Println("Переменные окружения: xrayCmd ещё не инициализирован")
	}

	xrayCmd.Stdout = os.Stdout
	xrayCmd.Stderr = os.Stderr

	if err := xrayCmd.Start(); err != nil {
		return fmt.Sprintf("Ошибка запуска: %v", err)
	}

	// Создаем файл блокировки
	if err := createLockFile(); err != nil {
		return fmt.Sprintf("Ошибка создания файла блокировки: %v", err)
	}

	// Сохраняем PID процесса в файл блокировки
	if err := writePidToLockFile(xrayCmd.Process.Pid); err != nil {
		if killErr := xrayCmd.Process.Kill(); killErr != nil {
			fmt.Printf("Ошибка при остановке процесса: %v\n", killErr)
		}
		if rmErr := removeLockFile(); rmErr != nil {
			fmt.Printf("Ошибка удаления файла блокировки: %v\n", rmErr)
		}
		return fmt.Sprintf("Ошибка записи PID: %v", err)
	}

	return fmt.Sprintf("▶️ Запуск Xray-core успешен")
}

// Останавливает Xray-core
func StopXray() string {
	if xrayCmd != nil && xrayCmd.Process != nil {
		fmt.Println("■ Остановка Xray-core...")
		err := xrayCmd.Process.Signal(syscall.SIGTERM)
		if err != nil {
			return fmt.Sprintf("❌ Не удалось отправить SIGTERM:", err)
		}
		if waitErr := xrayCmd.Wait(); waitErr != nil {
			return fmt.Sprintf("❌ Ошибка при ожидании завершения процесса:", waitErr)
		}
		// Обнуляем xrayCmd после успешной остановки
		xrayCmd = nil
	}
	return fmt.Sprintf("⏹️ Xray-core успешено отключен")
}

// Проверяет, запущен ли процесс xray
func IsXrayRunning() bool {
	// Проверяем наличие файла блокировки
	if _, err := os.Stat(lockFile); os.IsNotExist(err) {
		return false
	}

	// Читаем PID из файла блокировки
	pid, err := readPidFromLockFile()
	if err != nil {
		fmt.Printf("Ошибка чтения PID из файла блокировки: %v\n", err)
		return false
	}

	// Проверяем, существует ли процесс с этим PID
	if err := syscall.Kill(pid, 0); err == nil {
		return true
	}

	// Если процесс не существует, удаляем устаревший файл блокировки
	if rmErr := removeLockFile(); rmErr != nil {
		fmt.Printf("Ошибка удаления устаревшего файла блокировки: %v\n", rmErr)
	}
	return false
}

// Создает файл блокировки
func createLockFile() error {
	file, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("ошибка закрытия файла блокировки: %v", err)
	}
	return nil
}

// Удаляет файл блокировки
func removeLockFile() error {
	return os.Remove(lockFile)
}

// Записывает PID в файл блокировки
func writePidToLockFile(pid int) error {
	return os.WriteFile(lockFile, []byte(fmt.Sprintf("%d", pid)), 0644)
}

// Читает PID из файла блокировки
func readPidFromLockFile() (int, error) {
	data, err := os.ReadFile(lockFile)
	if err != nil {
		return 0, err
	}
	var pid int
	_, err = fmt.Sscanf(string(data), "%d", &pid)
	return pid, err
}
