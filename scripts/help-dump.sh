#!/bin/zsh
# Dump --help for every command, discovering subcommands via cobra's
# __complete (authoritative; immune to "Keys:"-style lines in Long text).
BIN=${BIN:-./admiral}
dump() {
  echo "=================================================================="
  echo "\$ admiral $* --help"
  echo "=================================================================="
  $BIN "$@" --help 2>&1
  echo
  $BIN __complete "$@" "" 2>/dev/null | awk -F'\t' '/^[a-z][a-z-]*\t/ {print $1}' | grep -v '^help$' | while read sub; do
    dump "$@" "$sub"
  done
}
dump
