default: build

.PHONY: clean mkbuild build multipassdev dpanel-build dev recovery test dbxdev

DPANEL_DIR ?= ../dpanel
DPANEL_DIST ?= $(DPANEL_DIR)/dist

clean:
	rm -rf ./build

mkbuild:
	mkdir -p build/

build: build/dogeboxd build/dbx build/_dbxroot

build/dogeboxd: clean mkbuild
	go build \
		-o build/dogeboxd \
			./cmd/dogeboxd/.

build/dbx: clean mkbuild
	go build \
		-o build/dbx \
		./cmd/dbx/.

build/_dbxroot: clean mkbuild
	go build \
		-o build/_dbxroot \
		./cmd/_dbxroot/.

multipassdev:
	go run ./cmd/dogeboxd -v -addr 0.0.0.0 -pups ~/

dpanel-build:
	@set -eu; \
	if command -v npm >/dev/null 2>&1; then \
		if [ ! -x "$(DPANEL_DIR)/node_modules/.bin/vite" ]; then \
			npm --prefix "$(DPANEL_DIR)" ci; \
		fi; \
		npm --prefix "$(DPANEL_DIR)" run build; \
	elif command -v nix >/dev/null 2>&1; then \
		DPANEL_DIR="$(DPANEL_DIR)" nix shell nixpkgs#nodejs_22 --command sh -lc '\
			cd "$$DPANEL_DIR" && \
			if [ ! -x node_modules/.bin/vite ]; then npm ci; fi && \
			npm run build \
		'; \
	else \
		echo "error: missing npm (and nix). Install Node/npm or prebuild $(DPANEL_DIST)" >&2; \
		exit 127; \
	fi

dev: build dpanel-build
	/run/wrappers/bin/dogeboxd -v --addr 0.0.0.0 --danger-dev \
		--data ~/data --nix ~/data/nix --containerlogdir ~/data/containerlogs \
		--port 3000 --uiport 8080 --uidir $(DPANEL_DIST) \
		--unix-socket ~/data/dbx-socket $(ARGS)

recovery:
	ARGS=--force-recovery make dev

test:
	go test -v ./test

create-loop-device:
	sudo truncate -s 512000000000 /loop0.img && sudo losetup /dev/loop0 /loop0.img

create-loop-device-2:
	sudo truncate -s 512000000000 /loop1.img && sudo losetup /dev/loop1 /loop1.img

delete-loop-device:
	sudo losetup -d /dev/loop0 && sudo rm /loop0.img

delete-loop-device-2:
	sudo losetup -d /dev/loop1 && sudo rm /loop1.img

dbxdev:
	DEV_DIR=~/data/dev DBX_SOCKET=~/data/dbx-socket DBX_CONTAINER_LOG_DIR=~/data/containerlogs go run ./cmd/dbx dev
