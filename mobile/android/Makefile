# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
# go-link Desk for Android. `make help` lists the targets.

# JDK 17 (brew install openjdk@17): Intel and Apple silicon Homebrew.
JAVA_HOME ?= $(firstword $(wildcard /opt/homebrew/opt/openjdk@17 /usr/local/opt/openjdk@17))
export JAVA_HOME
ADB ?= $(or $(wildcard $(ANDROID_HOME)/platform-tools/adb),$(wildcard $(HOME)/Library/Android/sdk/platform-tools/adb),adb)
# VERSION=1.0.0 sets the app's version and its version code
# (major*10000 + minor*100 + patch). Without it, the latest release tag.
VERSION ?= $(shell git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null | sed 's/^v//')
ifneq ($(VERSION),)
VERSION_FLAGS := -PversionName=$(VERSION) -PversionCode=$(shell echo '$(VERSION)' | awk -F. '{print $$1*10000 + $$2*100 + $$3}')
endif
GRADLE = ./gradlew $(VERSION_FLAGS)

.PHONY: help test-core strings debug install release bundle clean

help: ## This list
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

test-core: strings ## The portable logic's tests (JVM), and every text translated
	$(GRADLE) :core:test

strings: ## Writes the texts in English, Spanish and Portuguese (from the iOS app's)
	python3 scripts/strings.py

debug: ## app/build/outputs/apk/debug/app-debug.apk
	$(GRADLE) :app:assembleDebug

install: debug ## Installs the debug build on the connected device or emulator
	$(ADB) install -r app/build/outputs/apk/debug/app-debug.apk
	$(ADB) shell am start -n org.golink.desk/.MainActivity

release: ## A signed APK (DESK_SIGNING or keystore.properties; unsigned without it)
	$(GRADLE) :app:assembleRelease

bundle: ## A signed App Bundle for Google Play
	$(GRADLE) :app:bundleRelease

clean:
	$(GRADLE) clean
