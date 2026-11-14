{
  description = "tools-site — Go + htmx tool host";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "aarch64-darwin" "x86_64-darwin" ];
      forAll = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      devShells = forAll (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go            # 1.27.x (Go 1.25 is EOL — see the govulncheck finding)
            dart-sass     # `sass`, for `make css`
            kustomize     # deploy/k8s
            kubeconform   # manifest validation
            jq            # make list / make call
            curl
            git           # submodule
            watchexec     # make dev
            govulncheck   # make ci: fail on reachable CVEs
            gcc           # go test -race needs cgo
            nodejs        # scripts/webmcp.test.mjs
            gnumake
          ];
          env = {
            GOFLAGS = "-mod=readonly";
            # GOTOOLCHAIN stays auto on purpose: go.mod's toolchain directive pins the
            # patched patch release (1.27.4+) that nixpkgs does not ship yet.
          };
          shellHook = ''
            echo "tools-site dev shell: $(go version)"
            echo "  make dev    hot reload on :8080"
            echo "  make check  what CI runs, locally"
          '';
        };
      });
    };
}
