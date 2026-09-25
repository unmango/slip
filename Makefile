build:
	nix build .#

test:
	go test ./...

generate:
	nix run .#generate

# The Android archive the capture app links against. -androidapi has to match
# the SDK platform the dev shell pins, and gomobile will not infer it, so it
# comes from the same file the shell and the app read.
ANDROID_API := $(shell sed -n 's/^minSdk=//p' android/sdk-versions.properties)

bind: mobile/slip.aar
mobile/slip.aar: $(shell find internal mobile -name '*.go')
	gomobile bind -target=android -androidapi $(ANDROID_API) -o $@ ./mobile

# The capture app, around the archive above. There is no gradle wrapper: the
# dev shell pins gradle, so a wrapper would add a checked-in binary and a second
# version to keep in step.
apk: mobile/slip.aar
	nix develop .#android -c gradle assembleDebug

install: mobile/slip.aar
	nix develop .#android -c gradle installDebug

# The instrumented tests, on a headless emulator, against a git daemon the
# emulator can reach. This is the only thing that runs the app's code.
android-test: mobile/slip.aar
	nix develop .#android -c ./emulator-test.sh

update:
	nix flake update

check lint:
	nix flake check

format fmt:
	nix fmt

# Refresh vendorHash in nix/package.nix after a dependency change.
gomod:
	nix-update --flake --version=skip default
