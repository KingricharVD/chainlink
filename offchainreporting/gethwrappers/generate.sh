#!/usr/bin/env bash

# This builds very fast, but ignores inheritance dependencies between contracts,
# so it won't update TestOffchainAggregator if OffchainAggregator changes, for instance.

set -e

for solpath in ../../contract/src/*.sol; do
    solClass="$(basename $solpath .sol)"
    lowerCase="$(echo "$solClass" | tr '[:upper:]' '[:lower:]')"
    solpath="../../contract/src/${solClass}.sol"
    outdir="./$lowerCase"
    outpath="$outdir/${lowerCase}.go"
    (
        if [ "$solpath" -nt "$outpath" ]; then # Only rebuild if solpath newer
            echo "Rebuilding $solClass wrapper"
            mkdir -p "$outdir"
            abigen -sol "$solpath" -pkg "$lowerCase"  -out "$outpath"
            ./fix_metadata.py "$outpath"
            echo "Done rebuilding $solClass wrapper"
        fi
    ) & # Rebuild in parallel, if there is more than one wrapper to rebuild
done

wait
