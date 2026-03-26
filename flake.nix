{
  description = "Track, sync, and reproduce your software environment across Linux, macOS, and WSL2.";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
    in
    {
      packages = forAllSystems (system:
        let pkgs = nixpkgs.legacyPackages.${system}; in
        {
          default = pkgs.buildGoModule {
            pname = "genv";
            version = self.shortRev or self.dirtyShortRev or "dev";
            src = self;
            # Run `nix build` once; Nix will print the correct hash in the error.
            # Replace the value below with the "got:" hash from that output.
            vendorHash = pkgs.lib.fakeHash;
            ldflags = [
              "-s" "-w"
              "-X main.version=${self.shortRev or self.dirtyShortRev or "dev"}"
            ];
            meta = {
              description = "Track, sync, and reproduce your software environment across Linux, macOS, and WSL2.";
              homepage = "https://github.com/ks1686/genv";
              license = pkgs.lib.licenses.mit;
              mainProgram = "genv";
            };
          };
        });

      devShells = forAllSystems (system:
        let pkgs = nixpkgs.legacyPackages.${system}; in
        {
          default = pkgs.mkShell {
            buildInputs = with pkgs; [ go gopls golangci-lint ];
          };
        });
    };
}
