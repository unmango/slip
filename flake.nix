{
  description = "A zk wrapper and Zettelkasten tool suite";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    systems.url = "github:UnstoppableMango/nix-systems";

    flake-parts = {
      url = "github:hercules-ci/flake-parts";
      inputs.nixpkgs-lib.follows = "nixpkgs";
    };

    treefmt-nix = {
      url = "github:numtide/treefmt-nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    apis = {
      url = "github:unmango/apis";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.systems.follows = "systems";
      inputs.flake-parts.follows = "flake-parts";
      inputs.treefmt-nix.follows = "treefmt-nix";
    };

    a2b.follows = "apis/a2b";
  };

  outputs =
    inputs@{ flake-parts, ... }:
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = import inputs.systems;

      imports = with inputs; [
        systems.flakeModule
        treefmt-nix.flakeModule
      ];

      perSystem =
        {
          self',
          inputs',
          pkgs,
          lib,
          system,
          ...
        }:
        {
          packages.default = self'.packages.slip;
          packages.slip = pkgs.callPackage ./nix/package.nix { };

          # An attribute path cannot apply `.override`, so the zk-free build
          # needs an output of its own to be reachable from `nix build`.
          packages.slip-standalone = self'.packages.slip.override { withZk = false; };

          packages.generated = pkgs.callPackage ./nix/generated.nix {
            apisWorkspace = inputs'.apis.legacyPackages.unmangoApis.workspace;
            bufLib = inputs'.a2b.legacyPackages.lib.buf;
          };

          packages.generate = pkgs.callPackage ./nix/generate.nix {
            inherit (self'.packages) generated;
          };

          apps.generate = {
            program = lib.getExe self'.packages.generate;
            meta.description = "Codegen";
          };

          checks.vet = self'.packages.slip.overrideAttrs {
            checkPhase = ''
              runHook preCheck
              go vet ./...
              go test -race ./...
              runHook postCheck
            '';
          };

          devShells.default = pkgs.mkShellNoCC {
            packages = with pkgs; [
              buf
              gnumake
              go_1_27
              gofumpt
              gopls
              gotools
              nix-update
              nixfmt
              watchexec
            ];
          };

          # The Android SDK and NDK are unfree, and the SDK carries a licence
          # that has to be accepted before it will evaluate
          devShells.android =
            let
              pkgs = import inputs.nixpkgs {
                inherit system;
                config = {
                  allowUnfree = true;
                  android_sdk.accept_license = true;
                };
              };
            in
            pkgs.callPackage ./nix/shells/android.nix {
              inputsFrom = [ self'.devShells.default ];
            };

          treefmt.programs = {
            actionlint.enable = true;
            gofumpt.enable = true;
            mdformat.enable = true;
            nixfmt.enable = true;

            yamllint = {
              enable = true;
              settings.document-start = "disable";
            };

            zizmor.enable = true;
          };

          # Generated protobuf code is checked in verbatim. Formatting it would
          # put every regeneration permanently at odds with the generator.
          # Golden files are compared byte for byte, and a note is markdown whose
          # frontmatter fences mdformat reads as headings. Formatting either
          # tree rewrites the thing under test.
          treefmt.settings.global.excludes = [
            "gen/**"
            "**/testdata/**"
            "flake.lock"
            "LICENSE"
          ];
        };
    };
}
