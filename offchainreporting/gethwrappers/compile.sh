#!/usr/bin/env bash

set -e

solpath="$1"
solcoptions=("--optimize" "--optimize-runs" "40000")

basefilename="$(basename "$solpath" .sol)"
pkgname="$(echo $basefilename | tr '[:upper:]' '[:lower:]')"

here="$(dirname $0)"
pkgdir="${here}/${pkgname}"
mkdir -p "$pkgdir"
outpath="${pkgdir}/${pkgname}.go"
compiler_artifacts="${pkgdir}/${basefilename}.json"

solc "$solpath" ${solcoptions[@]} --combined-json abi,bin,bin-runtime,srcmap-runtime \
     > "$compiler_artifacts"

abigen --combined-json "$compiler_artifacts" -pkg "$pkgname"  -out "$outpath"

./fix_metadata.py "$outpath"
