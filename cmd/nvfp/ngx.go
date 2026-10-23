package main

// Registry layout is platform neutral so tests off Windows can pin it.

const (
	nvidiaRegGlobal = `SOFTWARE\NVIDIA Corporation\Global`
	ngxKeyName      = `NGXCore`
	ngxRegKey       = nvidiaRegGlobal + `\` + ngxKeyName
)

const defaultModelsDir = `C:\ProgramData\NVIDIA\NGX\models`
