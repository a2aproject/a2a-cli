# Turns commit subjects such as "feat(hook): add discovery" into release notes.
BEGIN { print "## What's new\n" }
match($0, /(feat|fix)(\([^)]*\))?!?: */) {
  note = substr($0, RSTART + RLENGTH)
  sub(/[ .]+$/, "", note)
  if (note == "") next
  print "- " toupper(substr(note, 1, 1)) substr(note, 2) "."
  n++
}
END {
  if (n == 0) print "- Maintenance and dependency updates."
  print "\nThanks to everyone who reported issues."
}
