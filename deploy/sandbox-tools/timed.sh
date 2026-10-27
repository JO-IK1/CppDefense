#!/bin/sh
# Each record is appended in a single write. Timings are diagnostics, not a
# security signal or evidence that a student's answer is correct.
phase=$1
shift
start=$(date +%s%N)
"$@"
code=$?
end=$(date +%s%N)
printf '%s %s\n' "$phase" "$((end-start))" >> /tmp/cppdefense-command-times
exit "$code"
