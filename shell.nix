with import <nixpkgs> { };
let
  basePackages = [
    gnumake
    gcc
    go_1_15
    go-tools
    gopls
    goimports
    (callPackage ./nix/pkgs/enumer.nix { })
  ];

  inputs = basePackages ++ lib.optional stdenv.isLinux inotify-tools;
  shellHook = "";

in mkShell {
  inherit shellHook;
  buildInputs = inputs;
}

