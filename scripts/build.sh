#!/usr/bin/env bash
#
# This script builds the application from source for multiple platforms.
# Based on the code from https://github.com/hashicorp/consul
set -e

export CGO_ENABLED=0

# TODO: Cross-compile for darwing and windows

# Get the parent directory of where this script is.
SOURCE="${BASH_SOURCE[0]}"
while [ -h "$SOURCE" ] ; do SOURCE="$(readlink "$SOURCE")"; done
DIR="$( cd -P "$( dirname "$SOURCE" )/.." && pwd )"

# Change into that directory
cd "$DIR"

GIT_COMMIT="$(git rev-parse HEAD)"
GO_LDFLAGS="-X github.com/umbracle/minimal/version.GitCommit=${GIT_COMMIT}"

# Determine the arch/os combos we're building for
XC_ARCH=${XC_ARCH:-"amd64"}
XC_OS=${XC_OS:-"linux"}

# Delete the old dir
echo "==> Removing old directory..."
rm -f bin/*
rm -rf pkg/*
mkdir -p bin/

# Build!
echo "==> Building..."
"`which gox`" \
    -os="${XC_OS}" \
    -arch="${XC_ARCH}" \
    -osarch="!darwin/arm !darwin/arm64" \
    -ldflags "${GO_LDFLAGS}" \
    -output "pkg/{{.OS}}_{{.Arch}}/minimal" \
    -tags="${GOTAGS}" \
    -cgo \
    .

# Move all the compiled things to the $GOPATH/bin
GOPATH=${GOPATH:-$(go env GOPATH)}
case $(uname) in
    CYGWIN*)
        GOPATH="$(cygpath $GOPATH)"
        ;;
esac
OLDIFS=$IFS
IFS=: MAIN_GOPATH=($GOPATH)
IFS=$OLDIFS

# Copy our OS/Arch to the bin/ directory
DEV_PLATFORM="./pkg/$(go env GOOS)_$(go env GOARCH)"
for F in $(find ${DEV_PLATFORM} -mindepth 1 -maxdepth 1 -type f); do
    cp ${F} bin/
    cp ${F} ${MAIN_GOPATH}/bin/
done

# Zip and copy to the dist dir
echo "==> Packaging..."
for PLATFORM in $(find ./pkg -mindepth 1 -maxdepth 1 -type d); do
    OSARCH=$(basename ${PLATFORM})
    echo "--> ${OSARCH}"

    pushd $PLATFORM >/dev/null 2>&1
    zip ../${OSARCH}.zip ./*
    popd >/dev/null 2>&1
done

# Done!
echo
echo "==> Results:"
ls -hl bin/
