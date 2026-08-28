//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const applicationName = "Lemmewatch"

func main() {
	if err := launch(); err != nil {
		showError(err)
		os.Exit(1)
	}
}

func launch() error {
	launcher, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate launcher: %w", err)
	}
	directory := filepath.Dir(launcher)
	tuiPath := filepath.Join(directory, "lemmewatch.exe")
	info, err := os.Stat(tuiPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("lemmewatch.exe was not found next to the launcher:\n%s", tuiPath)
		}
		return fmt.Errorf("inspect lemmewatch.exe: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("expected a file but found a directory:\n%s", tuiPath)
	}

	application, err := windows.UTF16PtrFromString(tuiPath)
	if err != nil {
		return fmt.Errorf("encode application path: %w", err)
	}
	arguments := append([]string{tuiPath}, os.Args[1:]...)
	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(arguments))
	if err != nil {
		return fmt.Errorf("encode command line: %w", err)
	}
	workingDirectory, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		return fmt.Errorf("encode working directory: %w", err)
	}
	startup := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var process windows.ProcessInformation
	if err := windows.CreateProcess(application, commandLine, nil, nil, false, windows.CREATE_NEW_CONSOLE, nil, workingDirectory, &startup, &process); err != nil {
		return fmt.Errorf("start lemmewatch.exe: %w", err)
	}
	windows.CloseHandle(process.Thread)
	windows.CloseHandle(process.Process)
	return nil
}

func showError(err error) {
	text, textErr := windows.UTF16PtrFromString(err.Error())
	title, titleErr := windows.UTF16PtrFromString(applicationName)
	if textErr != nil || titleErr != nil {
		return
	}
	_, _ = windows.MessageBox(0, text, title, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SETFOREGROUND)
}
