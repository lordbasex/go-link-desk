# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
#
# go-link Desk builds and checks.
#
#   make check        everything CI checks: Go, web and secrets
#   make web-build    build the website (frontend/apps/web/dist)
#   make device       the host for this computer (macOS app or Linux binary)
#   make e2e          end-to-end tests: signalhub + headless host + browser
#   make help         everything else

.PHONY: help info check check-go check-web check-secrets web-build web-deploy device device-headless \
	device-darwin-universal device-dmg device-notarized e2e clean FORCE

# The darwin and linux device targets are pattern rules (device-darwin-%,
# device-linux-%): make does not apply pattern rules to .PHONY targets, so
# they are not listed above.

# Project
PROJECT_NAME   = go-link-desk
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# macOS signature: "-" (ad hoc) is a development build that only runs on
# this Mac. Screen Recording and Accessibility permissions are tied to the
# signature, so an ad hoc build loses them on every rebuild: for daily
# development set CODESIGN_IDENTITY to a stable identity (a Developer ID,
# or a self-signed development certificate).
CODESIGN_IDENTITY ?= -
# The x.y.z macOS shows for the app: VERSION when it is one, else the
# latest vX.Y.Z tag, else 0.1.0.
SHORT_VERSION ?= $(shell echo '$(VERSION)' | grep -Eq '^v?[0-9]+\.[0-9]+\.[0-9]+$$' && echo '$(VERSION)' | sed 's/^v//' || git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null | sed 's/^v//' || true)
SHORT_VERSION := $(or $(SHORT_VERSION),0.1.0)
DIST           = $(CURDIR)/dist

SIGNAL_URL    ?= wss://signal.go-link.org/ws

# Paths
WEB_DIR      = frontend
WEB_DIST     = $(WEB_DIR)/apps/web/dist
DEVICE_DIR   = backend-device
DEVICE_OUT   = $(DIST)/desk
HOST_ARCH    = $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
HOST_OS      = $(shell uname -s | tr A-Z a-z)

# Tools run at a pinned version, without adding them to go.mod.
GOVULNCHECK  = golang.org/x/vuln/cmd/govulncheck@v1.1.4

# The Node version in frontend/.nvmrc, through nvm when it is installed.
NODE = cd $(WEB_DIR) && { [ -s "$$HOME/.nvm/nvm.sh" ] && . "$$HOME/.nvm/nvm.sh" >/dev/null && nvm use >/dev/null || true; } &&

# Output colors
GREEN  = \033[0;32m
YELLOW = \033[1;33m
RED    = \033[0;31m
NC     = \033[0m

.DEFAULT_GOAL := help

help:
	@echo "go-link Desk $(VERSION)"
	@echo ""
	@echo "  make check                  gofmt, vet, test -race, govulncheck, typecheck, vitest, web build, gitleaks"
	@echo "  make web-build              the website, in $(WEB_DIST)"
	@echo "  make web-deploy             publish the website to desk.go-link.org"
	@echo "  make device                 the host for this computer"
	@echo "  make device-headless        the host without a window (servers, CI)"
	@echo "  make device-darwin-universal  go-link-desk.app for Intel and Apple silicon"
	@echo "  make device-dmg             that app in a drag-to-Applications .dmg"
	@echo "  make device-notarized       signed with a Developer ID and notarized (source ~/.go-link-signing/env.sh first)"
	@echo "  make device-linux-amd64     the Linux host, built in Docker (also -arm64, -amd64-headless)"
	@echo "  make linux-package-amd64    that host with its installer, menu entry and icons, in a .tar.gz (also -arm64)"
	@echo "  make e2e                    Playwright against signalhub (../signaling) and a headless host"
	@echo "  make clean                  remove dist/"

info:
	@echo "$(GREEN)$(PROJECT_NAME) $(VERSION)$(NC)"
	@echo "Signaling:   $(SIGNAL_URL)"
	@echo "Host:        $(HOST_OS)/$(HOST_ARCH)"

# ---------------------------------------------------------------------------
# Checks (the same steps as .github/workflows/ci.yml)

check: check-go check-web check-secrets
	@echo "$(GREEN)✓ make check passed$(NC)"

check-go:
	@echo "$(YELLOW)▶ Go$(NC)"
	@cd $(DEVICE_DIR) && test -z "$$(gofmt -l .)" || { gofmt -l $(DEVICE_DIR); echo "$(RED)✗ gofmt: run gofmt -w$(NC)"; exit 1; }
	cd $(DEVICE_DIR) && go vet ./... && go vet -tags headless ./...
	cd $(DEVICE_DIR) && go test -race ./...
	cd $(DEVICE_DIR) && go run $(GOVULNCHECK) ./...

check-web:
	@echo "$(YELLOW)▶ Web$(NC)"
	$(NODE) npm ci --no-audit --no-fund
	$(NODE) npm run typecheck
	$(NODE) npm test
	$(NODE) npm run build
	@# The end-to-end tests' TypeScript, as CI checks it.
	cd e2e && { [ -s "$$HOME/.nvm/nvm.sh" ] && . "$$HOME/.nvm/nvm.sh" >/dev/null && nvm use "$$(cat ../$(WEB_DIR)/.nvmrc)" >/dev/null || true; } && npm ci --no-audit --no-fund >/dev/null && npx tsc -p .

check-secrets:
	@echo "$(YELLOW)▶ Secrets$(NC)"
	@if command -v gitleaks >/dev/null; then gitleaks git --no-banner --redact .; \
	else docker run --rm -v "$(CURDIR):/repo" ghcr.io/gitleaks/gitleaks:latest git /repo --no-banner --redact; fi

# ---------------------------------------------------------------------------
# Website

web-build:
	$(NODE) npm ci --no-audit --no-fund && \
		VITE_SIGNAL_URL=$(SIGNAL_URL) VITE_APP_VERSION=$(VERSION) npm run build
	@find $(WEB_DIST) -name '*.map' -delete
	@echo "$(GREEN)✓ $(WEB_DIST)$(NC)"

# Publishes the website to desk.go-link.org (S3 + CloudFront; see
# scripts/deploy-web.sh for the AWS profile it needs).
web-deploy: web-build
	scripts/deploy-web.sh $(WEB_DIST)

# ---------------------------------------------------------------------------
# Host

device: $(if $(filter darwin,$(HOST_OS)),device-darwin-$(HOST_ARCH),device-linux-$(HOST_ARCH))

# A quick headless build for this computer (servers, CI, e2e), linked
# against the libraries pkg-config finds: not a portable release.
device-headless:
	@mkdir -p $(DEVICE_OUT)
	cd $(DEVICE_DIR) && go build -tags headless -ldflags "-X main.version=$(VERSION)" -o $(DEVICE_OUT)/go-link-desk-headless ./cmd/desk
	@echo "$(GREEN)✓ $(DEVICE_OUT)/go-link-desk-headless$(NC)"

# macOS ships as an app bundle (go-link-desk.app): Screen Recording and
# Accessibility belong to the app's signature and bundle ID, not to a loose
# binary. clang builds both architectures on either kind of Mac, and
# build/macos/static-libs.sh builds libvpx and libopus from source for each
# one and for macOS $(MIN_MACOS), linked statically. -Wl,-w hides the
# linker's notes about the assembler objects, which are harmless.
MIN_MACOS      = 13.0
# GO_TAGS adds build tags to the macOS host (desk_test_screen: a measurement
# build that may accept viewers of the real screen without asking; never a
# release, which refuses it).
GO_TAGS       ?=
MACOS_LIBS     = $(DIST)/.macos-libs
MACOS_CLANG    = $$(xcrun -f clang)
clang_arch     = $(if $(filter amd64,$(1)),x86_64,arm64)

# The host binary of one architecture: dist/desk/darwin-<arch>/go-link-desk.
$(DEVICE_OUT)/darwin-%/go-link-desk: FORCE
	@if [ "$(HOST_OS)" != "darwin" ]; then echo "$(RED)✗ macOS builds need a Mac$(NC)"; exit 1; fi
	@echo "$(YELLOW)Building the host for darwin/$* (macOS $(MIN_MACOS)+)...$(NC)"
	@$(DEVICE_DIR)/build/macos/static-libs.sh $* $(MACOS_LIBS)/$*
	@mkdir -p $(DEVICE_OUT)/darwin-$*
	cd $(DEVICE_DIR) && GOOS=darwin GOARCH=$* CGO_ENABLED=1 \
		CC="$(MACOS_CLANG) -arch $(call clang_arch,$*)" \
		MACOSX_DEPLOYMENT_TARGET=$(MIN_MACOS) SDKROOT="$$(xcrun --sdk macosx --show-sdk-path)" \
		PKG_CONFIG_LIBDIR=$(MACOS_LIBS)/$*/lib/pkgconfig \
		CGO_CFLAGS="-O2 -mmacosx-version-min=$(MIN_MACOS)" \
		CGO_LDFLAGS="-mmacosx-version-min=$(MIN_MACOS) -Wl,-w" \
		go build -trimpath -tags "$(GO_TAGS)" -ldflags "-s -w -X main.version=$(VERSION)" \
		-o $(DEVICE_OUT)/darwin-$*/go-link-desk ./cmd/desk
	@scripts/check-deps.sh darwin $(DEVICE_OUT)/darwin-$*/go-link-desk

# One architecture's app, for development: dist/desk/darwin-<arch>/go-link-desk.app.
device-darwin-%: $(DEVICE_OUT)/darwin-%/go-link-desk
	@$(DEVICE_DIR)/build/macos/make-app.sh $(DEVICE_OUT)/darwin-$*/go-link-desk \
		$(DEVICE_OUT)/darwin-$*/go-link-desk.app $(VERSION) $(SHORT_VERSION) "$(CODESIGN_IDENTITY)"

# The release app: both architectures in one binary (lipo).
device-darwin-universal: $(DEVICE_OUT)/darwin-amd64/go-link-desk $(DEVICE_OUT)/darwin-arm64/go-link-desk
	@if echo "$(GO_TAGS)" | grep -q desk_test_screen; then echo "$(RED)✗ a release never has desk_test_screen$(NC)"; exit 1; fi
	@mkdir -p $(DEVICE_OUT)/darwin-universal
	lipo -create -output $(DEVICE_OUT)/darwin-universal/go-link-desk $(DEVICE_OUT)/darwin-amd64/go-link-desk $(DEVICE_OUT)/darwin-arm64/go-link-desk
	@$(DEVICE_DIR)/build/macos/make-app.sh $(DEVICE_OUT)/darwin-universal/go-link-desk \
		$(DEVICE_OUT)/darwin-universal/go-link-desk.app $(VERSION) $(SHORT_VERSION) "$(CODESIGN_IDENTITY)"

# The release app in a drag-to-Applications disk image:
# dist/desk/go-link-desk-<version>-macos-universal.dmg.
DMG = $(DEVICE_OUT)/go-link-desk-$(VERSION)-macos-universal.dmg
device-dmg: device-darwin-universal
	CODESIGN_IDENTITY="$(CODESIGN_IDENTITY)" $(DEVICE_DIR)/build/macos/make-dmg.sh $(DEVICE_OUT)/darwin-universal/go-link-desk.app $(DMG)

# Signed with a Developer ID and notarized by Apple, so it opens on any Mac
# without a warning: the app first (its ticket stapled into it), then the
# disk image made with it. CODESIGN_IDENTITY and NOTARY_PROFILE come from
# the environment (source ~/.go-link-signing/env.sh), never from the
# repository.
device-notarized:
	@test "$(CODESIGN_IDENTITY)" != "-" -a -n "$(NOTARY_PROFILE)" || { echo "$(RED)✗ set CODESIGN_IDENTITY (a Developer ID) and NOTARY_PROFILE first$(NC)"; exit 1; }
	@$(MAKE) device-darwin-universal
	NOTARY_PROFILE="$(NOTARY_PROFILE)" $(DEVICE_DIR)/build/macos/notarize.sh $(DEVICE_OUT)/darwin-universal/go-link-desk.app
	CODESIGN_IDENTITY="$(CODESIGN_IDENTITY)" $(DEVICE_DIR)/build/macos/make-dmg.sh $(DEVICE_OUT)/darwin-universal/go-link-desk.app $(DMG)
	NOTARY_PROFILE="$(NOTARY_PROFILE)" $(DEVICE_DIR)/build/macos/notarize.sh $(DMG)

FORCE:

# Linux builds in Docker (the other architecture through QEMU).
device-linux-%-headless:
	@echo "$(YELLOW)Building the headless host for linux/$*...$(NC)"
	cd $(DEVICE_DIR) && docker buildx build --platform linux/$* --build-arg TAGS=headless --build-arg VERSION=$(VERSION) \
		-f build/linux.Dockerfile --output type=local,dest=$(DEVICE_OUT)/linux-$*-headless .
	@echo "$(GREEN)✓ $(DEVICE_OUT)/linux-$*-headless/go-link-desk$(NC)"

device-linux-%:
	@echo "$(YELLOW)Building the host for linux/$*...$(NC)"
	cd $(DEVICE_DIR) && docker buildx build --platform linux/$* --build-arg VERSION=$(VERSION) \
		-f build/linux.Dockerfile --output type=local,dest=$(DEVICE_OUT)/linux-$* .
	@echo "$(GREEN)✓ $(DEVICE_OUT)/linux-$*/go-link-desk$(NC)"

# The Linux release: the host, its menu entry, icons and installer, in
# dist/desk/go-link-desk-<version>-linux-<arch>.tar.gz (and its .sha256).
LINUX_PKG = go-link-desk-$(VERSION)-linux-$*
linux-package-%: device-linux-%
	@rm -rf $(DEVICE_OUT)/$(LINUX_PKG) && mkdir -p $(DEVICE_OUT)/$(LINUX_PKG)
	cp $(DEVICE_OUT)/linux-$*/go-link-desk $(DEVICE_DIR)/build/linux/install.sh $(DEVICE_DIR)/build/linux/README.txt \
		$(DEVICE_DIR)/build/linux/org.go-link.desk.desktop LICENSE THIRD_PARTY_NOTICES.md $(DEVICE_OUT)/$(LINUX_PKG)/
	cd $(DEVICE_DIR) && GOOS= GOARCH= CGO_ENABLED=0 go run ./build/macos/appicon -linux $(abspath $(DEVICE_OUT))/$(LINUX_PKG)/icons
	COPYFILE_DISABLE=1 tar $(if $(filter darwin,$(HOST_OS)),--no-xattrs --no-mac-metadata) -C $(DEVICE_OUT) -czf $(DEVICE_OUT)/$(LINUX_PKG).tar.gz $(LINUX_PKG)
	cd $(DEVICE_OUT) && shasum -a 256 $(LINUX_PKG).tar.gz > $(LINUX_PKG).tar.gz.sha256
	@echo "$(GREEN)✓ $(DEVICE_OUT)/$(LINUX_PKG).tar.gz$(NC)"

# ---------------------------------------------------------------------------
# End-to-end tests (e2e/global-setup.ts builds signalhub, the web and the
# headless host itself)

e2e:
	cd e2e && npm ci --no-audit --no-fund && npx playwright install chromium && npx playwright test

clean:
	rm -rf $(DIST)
