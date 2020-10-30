with import <nixpkgs> { };
let
  # define packages to install with special handling for OSX
  basePackages = [ gnumake gcc go_1_15 go-tools gopls goimports ];
  inputs = basePackages ++ lib.optional stdenv.isLinux inotify-tools;

  shellHook = "";
  # define shell startup command
  # hooks = ''
  #   # this allows mix to work on the local directory
  #   mkdir -p .nix-mix
  #   mkdir -p .nix-hex
  #   export MIX_HOME=$PWD/.nix-mix
  #   export HEX_HOME=$PWD/.nix-hex
  #   export PATH=$MIX_HOME/bin:$PATH
  #   export PATH=$HEX_HOME/bin:$PATH
  #   export LANG=en_US.UTF-8
  #   export ERL_AFLAGS="-kernel shell_history enabled"
  # '';

in mkShell {
  inherit shellHook;
  buildInputs = inputs;
}

