#!/usr/bin/env bash

NUTCRACKER="$1"
if [ ! -f "$NUTCRACKER" ]; then
  echo "Nutcracker not found"
  exit 1
fi

OUTFILE="$2"
if [ -z "$OUTFILE" ]; then
  echo "Outfile is required"
  exit 1
fi

LOGFILE="$3"
if [ -z "$LOGFILE" ]; then
  echo "Logfile is required"
  exit 1
fi

killall -SIGKILL nutcracker
$NUTCRACKER -c "$OUTFILE" -o "$LOGFILE" -v 8 -d

exit $?
