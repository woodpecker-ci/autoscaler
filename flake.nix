{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=master";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    { nixpkgs, flake-utils, ... }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        devShells.default =
          with pkgs;
          let
            go = go_1_26;
          in
          pkgs.mkShell {
            buildInputs = [
              gnumake
              go
              glibc.static
              gofumpt
              golangci-lint
              go-mockery
              addlicense
              gcc
            ];
            CFLAGS = "-I${pkgs.glibc.dev}/include";
            LDFLAGS = "-L${pkgs.glibc}/lib";
            GO = "${go}/bin/go";
            GOROOT = "${go}/share/go";
          };
      }
    );
}
