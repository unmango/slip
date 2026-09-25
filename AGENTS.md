# Agent instructions

`slip` captures zettelkasten notes and passes anything else through to zk.
The binary is `slip`; the module is `github.com/UnstoppableMango/zettelkasten`.

## Layout

| Path | Owns |
| ------------------- | ---------------------------------------------------------------- |
| `android` | The Android capture app, in Kotlin and Compose |
| `cmd/slip` | Passthrough dispatch, then cobra |
| `internal/cli` | Cobra commands. Thin; behavior lives in the packages below |
| `internal/note` | The domain type and both serializations (frontmatter and proto) |
| `internal/store` | Writing notes to a directory. Knows nothing about their format |
| `internal/config` | Where notes go |
| `internal/gitsync` | Publishing captured notes to a git remote |
| `internal/notebook` | zk notebook discovery and `.zk/config.toml` |
| `internal/passthru` | Handing unowned commands to zk |
| `internal/tui` | The capture screen. Touches no files |
| `internal/zk` | Typed reader for `zk list --format jsonl` |
| `gen` | Generated protobuf. Never edited by hand |
| `mobile` | The gomobile binding surface. Thin; behavior lives in the packages above |

## Commands

| Command | Does |
| --------------- | ------------------------------------------------ |
| `make build` | `nix build .#` |
| `make test` | `go test ./...` |
| `make check` | `nix flake check` (treefmt, vet, race, tests) |
| `make fmt` | `nix fmt` |
| `make android-test` | Instrumented tests on a headless emulator |
| `make apk` | `gradle assembleDebug` in `android/` |
| `make bind` | `gomobile bind`, producing `mobile/slip.aar` for Android |
| `make install` | `gradle installDebug`, onto an attached device |
| `make generate` | Regenerate `gen/` from the pinned apis input |
| `make gomod` | Refresh `vendorHash` after a dependency change |

## Things that will otherwise be gotten wrong

**`gen/` is generated.** `packages.generated` builds the tree with `a2b`'s buf library against the buf workspace from the pinned `apis` flake input, which is what makes the `k8s.io/apimachinery` imports resolve without network or BSR. `make generate` copies that derivation into `gen/`, and `checks.generate` diffs the two, so `nix flake check` fails when the checked-in tree falls behind the pinned input. `gen/SOURCE` records the store path it came from. Never edit it; regenerate.

**`buf.gen.yaml` is the only template.** Both the dev shell's `buf generate` and `nix/generated.nix` read the in-tree file, so the config has one home. `protoc-gen-go` reaches the derivation through `nativeBuildInputs`, because `local: protoc-gen-go` resolves off `PATH`.

**The `a2b` input follows `apis/a2b`.** `apis` already depends on the same buf library, so following its node keeps one `a2b`, and its pulumi closure, in `flake.lock` rather than two.

**The generated protos use the protobuf opaque API.** There are no exported struct fields. `&notev1.Note{Title: ...}` will not compile. Build with `notev1.Note_builder{...}.Build()` and read with getters. Every builder field is a pointer, because these are edition 2024 files with explicit presence, so scalars are set with `proto.String(...)` and enums with `.Enum()`.

**Do not set `default_api_level` in `buf.gen.yaml`.** Edition 2024 already defaults to the opaque API. Forcing it would also flip the proto3 `google/api` and `k8s.io` files, diverging from what `unmango/apis` generates.

**treefmt excludes `gen/**` and `**/testdata/**`.** Both exclusions are load-bearing. gofumpt would rewrite generated files into permanent `make generate` drift, and mdformat reads a note's `---` frontmatter fences as markdown headings and rewrites golden files into garbage.

**Adding or removing a Go dependency requires `make gomod`.** Otherwise `nix build` fails. Note the failure is sometimes an "inconsistent vendoring" error rather than a hash mismatch, because nix reuses the cached vendor directory keyed by the stale hash.

**Global protobuf registry hazard.** Adding `google.golang.org/genproto/googleapis/api/annotations` or `k8s.io/api` as a dependency panics at init with "file already registered", because `gen/` registers those descriptors itself. Registry keys are proto file paths, so our own `go_package_prefix` does not avoid it. Init-time panic, no compile-time warning.

**A sync never merges and never rebases.**
go-git implements neither, and a zettelkasten does not need them: nothing is ever edited or deleted, so every publish is a fast-forward.
`gitsync` rewinds the worktree to the remote and replays the pending notes on top, holding them in memory for the duration, which is what makes a rejected push cost nothing and a sync interrupted between its commit and its push recoverable.

**A name match is not proof a note is published.**
Two devices capturing in the same minute produce different notes at the same address.
`gitsync.publishedAlready` compares blob hashes for that reason, and `commit` re-resolves the id through `note.NextID` against the remote, rewriting the frontmatter alongside the filename.
Weakening either one silently drops somebody's thought.

**The Android work needs `nix develop .#android`, not the default shell.**
The SDK, NDK, and a system image are several gigabytes of unfree closure, and none of it is needed to change a line of Go.

**There are two SDK roots, and mixing them up breaks the emulator confusingly.**
`ANDROID_HOME` is the build SDK, with the platforms and build tools the app compiles against.
`SLIP_EMULATOR_SDK` is the other one, which holds the emulator and its system image, composed separately because `composeAndroidPackages` fetches an image for every platform in `platformVersions`.
`avdmanager` and the emulator read `ANDROID_HOME` in preference to `ANDROID_SDK_ROOT`, so an AVD created against the build SDK records a system image path that does not exist, and the emulator dies saying the AVD is broken, which is not what is broken.

**Instrumented tests publish to `git daemon`, not to a directory.**
go-git speaks the git protocol in process, where a filesystem remote makes it exec `git-upload-pack`, and there is no git binary on an Android device.
`emulator-test.sh` starts the daemon on the host and passes `git://10.0.2.2:9418/...` in as an instrumentation argument.
Without that argument `PublishTest` skips rather than fails, so a bare `gradle connectedDebugAndroidTest` still runs everything it can.

**`mobile/slip.aar` is not checked in, and the app depends on it by path.**
It is 37M of compiled Go reproduced by `make bind` from the source beside it, so `make apk` needs a bind first.
Both Makefile targets declare that, but an IDE building `android/` on its own will not.

**`android/sdk-versions.properties` holds every SDK level, and four things read it.**
`nix/shells/android.nix` composes the platforms and build tools named there, `android/app/build.gradle.kts` parses it with `java.util.Properties`, the Makefile greps `minSdk` out of it for `-androidapi`, and `emulator-test.sh` falls back to `compileSdk` for its system image.
It is a properties file because Gradle parses it with nothing but the JDK, and the Nix side reads it rather than the reverse so that an IDE building `android/` alone still gets the right numbers.
Change a level there and nowhere else.
`minSdk` must equal the `-androidapi` gomobile bound against, because the archive is compiled for that NDK sysroot and a lower `minSdk` links against symbols the device will not have.
`compileSdk` and `buildTools` must be platforms the flake composes, because AGP responds to a missing one by trying to install it, and the SDK is read-only in the nix store.

**The APK is split per ABI, so there is no `app-debug.apk`.**
The bulk of this app is `libgojni.so`, and a universal build carries four copies of the Go runtime where three can never run.
The outputs are `app-arm64-v8a-debug.apk` and its siblings, about 28M each against 82M universal.
`gradle installDebug` picks the right one for whatever is attached; anything scripting `adb install` has to name the ABI.

**There is no gradle wrapper, deliberately.**
The dev shell pins gradle, so a wrapper would add a checked-in binary and a second version to keep in step.
Build with `make apk` or `nix develop -c gradle`, never a bare `./gradlew`.

**Every sync goes through `SyncWorker`, including the one the person taps.**
`mobile.Client` is built fresh wherever one is needed, so its sync lock is package level in Go rather than a field.
Routing the UI through WorkManager's unique work is the other half: two syncs on one worktree is not a state the rewind-and-replay model survives.

**`mobile/bind.go` is why `go mod tidy` keeps `golang.org/x/mobile`.**
gomobile resolves its bind runtime through the module graph, so the module has to require it even though nothing here imports it.
Build constraints do not hide an import from tidy, which is what makes a file nothing compiles work as the anchor.
Delete it and the next `make bind` fails with "no required module provides package".

**`-androidapi` has to match the SDK platform the dev shell pins.**
gomobile compiles against `platforms/android-<androidapi>` and refuses to run when it is absent, and it will not infer the level from what is installed.
Both the flake and the Makefile take it from `minSdk` in `android/sdk-versions.properties`, so there is one number to change.
The NDK accepts 21 through 35, so the default of 16 fails outright.

**The Android SDK is unfree and its licence has to be accepted at evaluation time.**
`flake.nix` imports a second nixpkgs with `allowUnfree` and `android_sdk.accept_license` for that alone, so no other output and nothing a consumer builds from this flake inherits that config.

**`mobile` is constrained by what gomobile can bind.**
Strings, numbers, errors, and structs of those.
No slices of structs, no maps, no channels, no exported field holding an interface.
`mobile.TestBindableSurface` pins the signatures, because the alternative is finding out during an Android build.

**`nix/package.nix` needs `git` in `nativeCheckInputs`, and every new top-level directory in its fileset.**
The gitsync tests publish to a remote that is a directory on disk, and go-git serves that transport by executing `git-upload-pack` rather than in process.
The fileset is an allowlist: a package missing from it fails `nix flake check` at vet with "cannot find module providing package", which reads like a vendoring problem and is not one.

**zk-org/zk is GPL-3.0-only and this repo is MIT.** Never copy, vendor, or import its code; all of it is under `internal/` and unimportable anyway. Interop is subprocess-only: run the binary, parse its output. Relicensing would be the prerequisite for changing that, and it is not a thing to do incidentally.

**zk's JSON output is unversioned.** `internal/zk/testdata/list.jsonl` is real output captured from zk and is the only thing standing between a zk upgrade and a silent breakage. Regenerate it deliberately, by running slip inside a real notebook, not by hand-editing.

## Contracts worth preserving

The body is stored **verbatim**. The title is derived from the first line, never cut out of it. Nothing a person typed is moved or deleted.

`zettel_id` is both the frontmatter field and the filename stem. `store.Create` resolves collisions by updating both together; they must never drift apart.

Output-only proto fields are never written by capture: `uid`, `update_time`, `delete_time`, `last_edited_time`, and every count. They belong to whatever serves the resource, and the counts come from `zk list` / `zk graph`.

`internal/tui` never touches the filesystem. It returns a `Result` and the caller writes. That is what makes it testable without a terminal, so keep it that way.

The owned surface is deliberately small, because everything in it shadows zk. A command name `slip` claims is a zk command it hides, and a flag it registers on the root command is a zk global flag it hides. `passthru.Owns` decides ownership by looking the leading option up in `cli.Flags()`, so registering a new root flag silently takes that name away from zk; `internal/passthru/passthru_test.go` pins the zk options that must keep passing through.
