#!/usr/bin/env bash
# Run from the repository root on Linux x86_64. Never overwrite the checked-in file.
set -euo pipefail

tools=$(mktemp -d)
trap 'rm -rf "$tools"' EXIT
curl --fail --silent --show-error --location --retry 3 \
  https://github.com/protocolbuffers/protobuf/releases/download/v3.20.3/protoc-3.20.3-linux-x86_64.zip \
  -o "$tools/protoc.zip"
(cd "$tools" && echo '44a6b498e996b845edef83864734c0e52f42197e85c9d567af55f4e3ff09d755  protoc.zip' | sha256sum --check && unzip -q protoc.zip)
# The generator needs Go >= 1.23; this override does not change the project's build toolchain.
GOBIN="$tools/bin" GOTOOLCHAIN=go1.23.12 go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.10
mkdir "$tools/out"
cd internal/serialization
"$tools/bin/protoc" -I . -I "$tools/include" --plugin="protoc-gen-go=$tools/bin/protoc-gen-go" \
  --go_out="$tools/out" --go_opt=paths=source_relative v1.proto
diff -u v1.pb.go "$tools/out/v1.pb.go"
