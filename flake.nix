{
  description = "loqui dev env";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      devShells = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go_1_26
              gopls
              gotools
              go-tools
              golangci-lint
              delve
              postgresql_16
              docker-compose
              fish
            ];

            GOTOOLCHAIN = "local";

            shellHook = ''
              echo "loqui dev shell  ·  $(go version)"
 
              if [[ $- == *i* && -z ''${IN_LOQUI_SHELL:-} ]]; then
                export IN_LOQUI_SHELL=1
                export SHELL="$(command -v fish)"
                exec fish
              fi
            '';
          };
        }
      );
    };
}
