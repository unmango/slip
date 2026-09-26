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
        let
          androidPkgs = import inputs.nixpkgs {
            inherit system;
            config = {
              allowUnfree = true;
              android_sdk.accept_license = true;
            };
          };
        in
        {
          packages = {
            default = self'.packages.slip;
            slip = pkgs.callPackage ./nix/package.nix { };

            slip-standalone = self'.packages.slip.override {
              withZk = false;
            };

            generate = pkgs.callPackage ./nix/generate.nix {
              generated = pkgs.callPackage ./nix/generated.nix {
                apisWorkspace = inputs'.apis.legacyPackages.unmangoApis.workspace;
                bufLib = inputs'.a2b.legacyPackages.lib.buf;
              };
            };
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

          devShells.android = androidPkgs.callPackage ./nix/shells/android.nix {
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

          treefmt.settings.global.excludes = [
            "gen/**"
            "**/testdata/**"
            "flake.lock"
            "LICENSE"
          ];
        };
    };
}
