#!/usr/bin/env bash
# Нагрузочный прогон одного сценария через hey и сводка в CSV.
# Использование: scripts/load.sh <имя> <url> [клиентов=20] [секунд=15]
set -euo pipefail
NAME=$1; URL=$2; C=${3:-20}; DUR=${4:-15}
HEY=${HEY:-$(go env GOPATH)/bin/hey}
OUT=results/load; mkdir -p "$OUT"
"$HEY" -z "${DUR}s" -c "$C" -t 5 "$URL" > "$OUT/$NAME.txt"

awk -v name="$NAME" -v c="$C" -f scripts/summarize.awk "$OUT/$NAME.txt" | tee -a results/summary.csv
