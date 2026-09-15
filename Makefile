WINDRES ?= x86_64-w64-mingw32-windres
RC       = nvfp.rc
RES_OBJ  = nvfp_res_windows_amd64.syso

# The resource object is committed, so the build needs no MinGW toolchain.
# When windres is installed, rebuild it automatically if the icon sources change.
RES_DEP := $(if $(shell command -v $(WINDRES) 2>/dev/null),$(RES_OBJ),)

build: $(RES_DEP)
	@test -f $(RES_OBJ) || { echo "$(RES_OBJ) missing: run 'make resources' (needs $(WINDRES))"; exit 1; }
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="-s -w -buildid=" -o nvfp.exe .

# Rebuild the Windows resource object (application icon) from $(RC).
resources: $(RES_OBJ)

$(RES_OBJ): $(RC) nvfp.ico
	$(WINDRES) -i $(RC) -o $@ -O coff --target=pe-x86-64

test:
	go test ./...

.PHONY: build resources test
