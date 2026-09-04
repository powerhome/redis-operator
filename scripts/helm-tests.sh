#!/bin/bash

set -eu

charts=(
  charts/redisoperator
  charts/redisfailover-bundle
)

for chart in "${charts[@]}"; do
  echo ">> Testing chart ${chart}"
  helm lint "${chart}"
  helm template "${chart}"
  echo "> Chart OK"
done
