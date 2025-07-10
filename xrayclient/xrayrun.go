package xrayclient

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

var xrayCmd *exec.Cmd

// Пути к исполняемому файлу и конфигу
const (
	xrayExecutable = "/home/andrew/distr/github_copy/Xray-core"
	xrayConfigDir  = "/home/andrew/distr/github_copy/Xray-core/mysettings/config"
)

// Запускает Xray-core
func StartXray() {
	xrayCmd = exec.Command(xrayExecutable, "run", "-c", xrayConfigDir)
	xrayCmd.Stdout = os.Stdout
	xrayCmd.Stderr = os.Stderr

	fmt.Println("▶ Запуск Xray-core...")
	if err := xrayCmd.Start(); err != nil {
		fmt.Println("❌ Ошибка запуска:", err)
	}
}

// Останавливает Xray-core
func StopXray() {
	if xrayCmd != nil && xrayCmd.Process != nil {
		fmt.Println("■ Остановка Xray-core...")
		err := xrayCmd.Process.Signal(syscall.SIGTERM)
		if err != nil {
			fmt.Println("❌ Не удалось отправить SIGTERM:", err)
		}
		xrayCmd.Wait() // Ждём завершения процесса
	}
}

// Главный цикл
func main() {
	// Обработка Ctrl+C и завершения
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigs
		StopXray()
		fmt.Println("\n⛔ Программа завершена.")
		os.Exit(0)
	}()

	StartXray()

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Введите 'restart' для перезапуска или 'exit' для выхода.")

	for scanner.Scan() {
		command := scanner.Text()
		switch command {
		case "restart":
			StopXray()
			StartXray()
		case "exit":
			StopXray()
			fmt.Println("⛔ Выход.")
			return
		default:
			fmt.Println("Неизвестная команда. Доступные: restart, exit.")
		}
	}
}
