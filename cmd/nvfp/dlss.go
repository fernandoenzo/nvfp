package main

import "fmt"

const (
	dlssIndicatorName = "ShowDlssIndicator"
	dlssDebugOn       = 1024
	dlssDebugOff      = 0
)

const dlssRestartHint = "Note: restart any running game for the indicator to appear; the driver reads it when the game initialises DLSS."

// dlssRegistry is injected so the flow runs off Windows. found is false when
// the key or the value is absent, which is not an error.
type dlssRegistry struct {
	ReadValue  func() (value uint32, found bool, err error)
	WriteValue func(value uint32) error
}

func runSetDLSSDebug(enabled bool) error {
	if elevatedFlag && !stdoutIsPiped() {
		defer pauseBeforeExit()
	}
	return setDLSSDebug(enabled, ngxDlssRegistry())
}

// setDLSSDebug creates the key when it is missing; a read failure aborts before
// the UAC prompt, because the driver reads the value from the unelevated game
// process and elevation would not fix a read that fails anyway.
func setDLSSDebug(enabled bool, reg dlssRegistry) error {
	target := dlssTarget(enabled)
	current, found, err := reg.ReadValue()
	if err != nil {
		return err
	}
	switch {
	case found && current == target:
		fmt.Printf("Nothing to do: %s is already %d (%s)\n", dlssIndicatorName, target, dlssState(target))
		return nil
	case dryRun:
		fmt.Printf("Would set %s = %d (%s)\n", dlssIndicatorName, target, dlssState(target))
		return nil
	}
	ensureElevated(true)
	if err := reg.WriteValue(target); err != nil {
		return fmt.Errorf("writing %s: %w", dlssIndicatorName, err)
	}
	fmt.Printf("Set %s = %d (%s)\n", dlssIndicatorName, target, dlssState(target))
	if enabled {
		fmt.Println(dlssRestartHint)
	}
	return nil
}

func dlssTarget(enabled bool) uint32 {
	if enabled {
		return dlssDebugOn
	}
	return dlssDebugOff
}

func dlssState(value uint32) string {
	if value == dlssDebugOn {
		return "DLSS debug indicator on"
	}
	return "DLSS debug indicator off"
}
