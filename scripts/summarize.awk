# Сводка вывода hey: name,clients,rps,avg_ms,p50_ms,p95_ms,p99_ms,error_pct,requests
/Requests\/sec/          {rps=$2}
/Average:/ && avg==""    {avg=$2*1000}
/ 50%+ in/               {p50=$3*1000}
/ 95%+ in/               {p95=$3*1000}
/ 99%+ in/               {p99=$3*1000}
/Error distribution/     {inerr=1; next}
/^ *\[[0-9]+\]/ {
  n=$1; gsub(/[\[\]]/,"",n)
  if (inerr) {total+=n; bad+=n}
  else {code=n; cnt=$2; total+=cnt; if (code!=200) bad+=cnt}
}
END {
  err = total ? 100*bad/total : 0
  printf "%s,%d,%.1f,%.2f,%.2f,%.2f,%.2f,%.2f,%d\n", name, c, rps, avg, p50, p95, p99, err, total
}
