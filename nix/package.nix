{
  lib,
  buildGoModule,
  git,
  go_1_27,
  makeWrapper,
  zk,
  # slip hands any command it does not implement to zk. Wrapping PATH makes that
  # work out of the box; disable it for a binary with no zk in its closure.
  #
  # zk is GPL-3.0 and slip is MIT. Putting a program on PATH is invoking it, not
  # linking against it, so this stays an arms-length boundary.
  withZk ? true,
}:
(buildGoModule.override { go = go_1_27; }) (finalAttrs: {
  pname = "slip";
  version = "0.1.0";

  src = lib.fileset.toSource {
    root = ../.;
    fileset = lib.fileset.unions [
      ../go.mod
      ../go.sum
      ../cmd
      ../gen
      ../internal
      ../mobile
    ];
  };

  vendorHash = "sha256-cA/JXLCpTC+Hm3NwYSj3aCCnrkdnOm0VFbrllA5JBQ8=";

  subPackages = [ "cmd/slip" ];

  nativeBuildInputs = lib.optional withZk makeWrapper;

  # gitsync's tests publish to a remote that is a directory on disk, and go-git
  # serves that transport by executing git-upload-pack rather than in process.
  # Nothing but the tests needs it, and it stays out of the runtime closure.
  nativeCheckInputs = [ git ];

  postInstall = lib.optionalString withZk ''
    wrapProgram $out/bin/slip --suffix PATH : ${lib.makeBinPath [ zk ]}
  '';

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${finalAttrs.version}"
  ];

  meta = {
    description = "Capture zettelkasten notes";
    homepage = "https://github.com/unmango/slip";
    license = lib.licenses.mit;
    mainProgram = "slip";
  };
})
