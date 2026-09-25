# The Android dev shell, separate from the default one because the SDK, NDK,
# and a system image are several gigabytes of unfree closure and none of it is
# needed to edit Go code.
{
  androidenv,
  git,
  gomobile,
  gradle,
  inputsFrom ? [ ],
  jdk17,
  lib,
  mkShellNoCC,
}:
let
  # The levels come from android/sdk-versions.properties so that this shell and
  # the app agree by construction. gomobile compiles against the SDK platform
  # matching its -androidapi and refuses to run when that platform is absent,
  # and AGP responds to a missing compileSdk by trying to install it into the
  # read-only store, so composing anything but what the app asks for breaks a
  # build rather than a shell.
  sdk =
    let
      lines = lib.filter (l: l != "" && !lib.hasPrefix "#" l) (
        lib.splitString "\n" (builtins.readFile ../../android/sdk-versions.properties)
      );
    in
    lib.listToAttrs (
      map (
        l:
        let
          parts = lib.splitString "=" l;
        in
        lib.nameValuePair (lib.head parts) (lib.concatStringsSep "=" (lib.tail parts))
      ) lines
    );

  androidApi = sdk.minSdk;
  androidCompileApi = sdk.compileSdk;

  androidBuild = androidenv.composeAndroidPackages {
    includeNDK = true;
    platformVersions = [
      androidApi
      androidCompileApi
    ];
    buildToolsVersions = [ sdk.buildTools ];
  };

  # composeAndroidPackages fetches a system image for every platform in
  # platformVersions, so the emulator is composed on its own. Sharing
  # androidBuild's list would mean a second image, for an Android nothing is
  # tested on, at a gigabyte and a half.
  androidEmulator = androidenv.composeAndroidPackages {
    includeEmulator = true;
    includeSystemImages = true;
    systemImageTypes = [ "google_apis" ];
    abiVersions = [ "x86_64" ];
    platformVersions = [ androidCompileApi ];
  };
in
mkShellNoCC {
  inherit inputsFrom;

  packages = [
    # gomobile builds mobile/ into an .aar. Its wrapper puts the SDK on PATH
    # and sets ANDROID_HOME; the JDK is what assembles the archive once the NDK
    # has compiled the Go side, and what Gradle runs on to build the app around
    # it.
    (gomobile.override { androidPkgs = androidBuild; })
    gradle
    jdk17

    # git daemon serves the notebook the instrumented tests publish to. go-git
    # speaks that protocol in process, where a remote on the local filesystem
    # would need git-upload-pack on the device.
    git
  ];

  # Gradle finds the SDK through ANDROID_HOME. gomobile's wrapper sets the same
  # variable for itself, but only inside its own process, so Gradle needs it
  # here.
  ANDROID_HOME = "${androidBuild.androidsdk}/libexec/android-sdk";

  # The emulator and its system image live in their own SDK root, and
  # android/emulator-test.sh reads this to find them.
  SLIP_EMULATOR_SDK = "${androidEmulator.androidsdk}/libexec/android-sdk";
  SLIP_ANDROID_API = androidCompileApi;

  # gomobile's wrapper appends its own store path to GOPATH, so an unset GOPATH
  # leaves the read-only store as the only entry and the module cache has
  # nowhere to go. Setting Go's own default here puts a writable directory in
  # front of it.
  shellHook = ''
    export GOPATH="''${GOPATH:-$HOME/go}"
    cd "$(git rev-parse --show-toplevel)/android"
  '';
}
