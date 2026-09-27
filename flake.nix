{
  description = "datalog-poc: a minimal Mangle/Datalog learning project";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";

  outputs = { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };
    in {
      devShells.${system}.default = pkgs.mkShell {
        # go is the only tool this project needs: it fetches the Mangle
        # library itself via `go mod tidy` / `go build`.
        packages = [ pkgs.go ];
      };
    };
}
