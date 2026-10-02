//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// errElevationCancelled reports that the user dismissed the UAC prompt.
var errElevationCancelled = errors.New("elevation cancelled")

// isElevated reports whether the current process runs with an elevated token.
// Writing the driver profile database requires administrator privileges.
func isElevated() bool {
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return false
	}
	defer token.Close()
	var elevated uint32
	if err := syscall.GetTokenInformation(token, uint32(syscall.TokenElevation),
		(*byte)(unsafe.Pointer(&elevated)), 4, nil); err != nil {
		return false
	}
	return elevated != 0
}

// shellExecuteInfoW mirrors SHELLEXECUTEINFOW (112 bytes on amd64), verified
// against shellapi.h.
type shellExecuteInfoW struct {
	cbSize         uint32
	fMask          uint32
	hwnd           uintptr
	lpVerb         *uint16
	lpFile         *uint16
	lpParameters   *uint16
	lpDirectory    *uint16
	nShow          int32
	hInstApp       uintptr
	lpIDList       uintptr
	lpClass        *uint16
	hkeyClass      uintptr
	dwHotKey       uint32
	hIconOrMonitor uintptr
	hProcess       uintptr
}

// The layout is hand-written and handed to the shell by address, so it is
// pinned at compile time against the offsets from shellapi.h.
var (
	_ [unsafe.Sizeof(shellExecuteInfoW{}) - 112]struct{}               = [0]struct{}{}
	_ [unsafe.Offsetof(shellExecuteInfoW{}.fMask) - 4]struct{}         = [0]struct{}{}
	_ [unsafe.Offsetof(shellExecuteInfoW{}.lpVerb) - 16]struct{}       = [0]struct{}{}
	_ [unsafe.Offsetof(shellExecuteInfoW{}.lpFile) - 24]struct{}       = [0]struct{}{}
	_ [unsafe.Offsetof(shellExecuteInfoW{}.lpParameters) - 32]struct{} = [0]struct{}{}
	_ [unsafe.Offsetof(shellExecuteInfoW{}.lpDirectory) - 40]struct{}  = [0]struct{}{}
	_ [unsafe.Offsetof(shellExecuteInfoW{}.nShow) - 48]struct{}        = [0]struct{}{}
	_ [unsafe.Offsetof(shellExecuteInfoW{}.hProcess) - 104]struct{}    = [0]struct{}{}
)

var (
	modShell32          = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteExW = modShell32.NewProc("ShellExecuteExW")
)

// relaunchElevated restarts the program through the UAC prompt with the same
// arguments plus --elevated, waits for the child and returns its exit code.
// It returns errElevationCancelled when the user declines the prompt.
func relaunchElevated() (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("locating the executable: %w", err)
	}
	// A relative --games-json must keep working in the child, so the child
	// starts in the current directory.
	dir, err := os.Getwd()
	if err != nil {
		return 0, fmt.Errorf("getting the working directory: %w", err)
	}
	verb, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return 0, err
	}
	file, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return 0, err
	}
	params, err := syscall.UTF16PtrFromString(quoteArgs(append(os.Args[1:], "--elevated")))
	if err != nil {
		return 0, err
	}
	cwd, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}

	const (
		seeMaskNoCloseProcess = 0x40
		swShowNormal          = 1
		errorCancelled        = 1223
	)
	sei := shellExecuteInfoW{
		fMask:        seeMaskNoCloseProcess,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: params,
		lpDirectory:  cwd,
		nShow:        swShowNormal,
	}
	sei.cbSize = uint32(unsafe.Sizeof(sei))

	ret, _, errno := syscall.SyscallN(procShellExecuteExW.Addr(), uintptr(unsafe.Pointer(&sei)))
	if ret == 0 {
		if errno == errorCancelled {
			return 0, errElevationCancelled
		}
		return 0, fmt.Errorf("ShellExecuteExW failed: %w", errno)
	}
	process := syscall.Handle(sei.hProcess)
	if process == 0 {
		return 0, errors.New("ShellExecuteExW returned no process handle")
	}
	defer syscall.CloseHandle(process)
	syscall.WaitForSingleObject(process, syscall.INFINITE)
	var code uint32
	if err := syscall.GetExitCodeProcess(process, &code); err != nil {
		return 0, fmt.Errorf("reading the elevated process exit code: %w", err)
	}
	return int(code), nil
}

// quoteArgs renders the command line for the elevated child, quoting each
// argument the way CommandLineToArgvW expects.
func quoteArgs(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, syscall.EscapeArg(arg))
	}
	return strings.Join(quoted, " ")
}

// stdoutIsPiped reports whether the output is redirected, in which case a
// pause would block a script rather than let a human read the window.
func stdoutIsPiped() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice == 0
}

// pauseBeforeExit keeps the elevated child's console window open until the user
// presses Enter, so a run that finishes in milliseconds can actually be read.
func pauseBeforeExit() {
	fmt.Fprint(os.Stderr, "\nPress Enter to close this window...")
	fmt.Scanln()
}
