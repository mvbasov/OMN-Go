#!/bin/bash
SEARCH_WHERE="backend/frontend/html/js/omn-go-core.js backend/frontend/html/js/omn-go-sse.js"

# Define color codes for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}\`async\`${NC} found in:"
grep -i -e 'async' ${SEARCH_WHERE}
echo -e "${GREEN}\`/api/\`${NC} found in:"
grep -i -e '\/api\/' ${SEARCH_WHERE}

grep -r -B 20 --color=always "fetch(" --include="*.js" . | awk '
  /^--$/ { next }
  /^[[:space:]]*function/ { func=$0; next }
  /^[[:space:]]*const/    { func=$0; next }
  /^[[:space:]]*let/      { func=$0; next }
  /^[[:space:]]*var/      { func=$0; next }
  /fetch\(/ {
    if (func != "") {
      print func
      func=""
    }
    print
  }
'
