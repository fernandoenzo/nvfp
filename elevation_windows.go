//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// errElevationCancelled reports that the user dismissed the UAC prompt.
var errElevationCancelled = errors.New("elevation cancelled")

// isElevated reports whether the current process runs with an elevated token.
// Writing the driver profile database requires administrator privileges. It
// delegates to x/sys, whose Token.IsElevated is the well-tested implementation:
// a hand-rolled version that passed a null ReturnLength to GetTokenInformation
// failed with ERROR_INVALID_PARAMETER and silently reported every process as
// unelevated, so the driver step was always skipped. The token query is not
// silent either way: a failure is reported instead of read as "not an admin".
func isElevated() (bool, error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return false, fmt.Errorf("opening the process token: %w", err)
	}
	defer token.Close()
	return token.IsElevated(), nil
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

// shell32.dll lives in Windows\System32, so the system-only variant is used,
// as in internal/nvdr for nvapi64.dll: it restricts the search to that
// directory instead of walking the normal search order. shell32.dll happens to
// sit in the stdlib's internal system-DLL allowlist, but syscall.LoadDLL still
// points at x/sys as the supported way to load a system DLL.
var (
	modShell32          = windows.NewLazySystemDLL("shell32.dll")
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
	sei, err := elevatedLaunchInfo(exe, dir)
	if err != nil {
		return 0, err
	}
	return runElevated(sei)
}

// elevatedLaunchInfo builds the request that asks the shell to relaunch exe as
// an administrator, with the same arguments plus --elevated, from dir.
func elevatedLaunchInfo(exe, dir string) (*shellExecuteInfoW, error) {
	const (
		seeMaskNoCloseProcess = 0x40
		swShowNormal          = 1
	)
	sei := &shellExecuteInfoW{fMask: seeMaskNoCloseProcess, nShow: swShowNormal}
	var err error
	if sei.lpVerb, err = syscall.UTF16PtrFromString("runas"); err != nil {
		return nil, err
	}
	if sei.lpFile, err = syscall.UTF16PtrFromString(exe); err != nil {
		return nil, err
	}
	args := quoteArgs(append(os.Args[1:], "--elevated"))
	if sei.lpParameters, err = syscall.UTF16PtrFromString(args); err != nil {
		return nil, err
	}
	if sei.lpDirectory, err = syscall.UTF16PtrFromString(dir); err != nil {
		return nil, err
	}
	sei.cbSize = uint32(unsafe.Sizeof(*sei))
	return sei, nil
}

// runElevated hands the request to the shell and waits for the elevated child,
// returning its exit code. It returns errElevationCancelled when the user
// declines the prompt.
func runElevated(sei *shellExecuteInfoW) (int, error) {
	ret, _, errno := procShellExecuteExW.Call(uintptr(unsafe.Pointer(sei)))
	// Call returns a plain error built from GetLastError; errors.Is unwraps it
	// to the windows.Errno the comparison needs.
	if ret == 0 {
		if errors.Is(errno, windows.ERROR_CANCELLED) {
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
