#!/usr/bin/env bash
# install.sh — install fsearch into ~/.local/bin for everyday shell use.
#
# Why ~/.local/bin?
#   Fixed per-user location on Linux (not tied to GOBIN/GOPATH).
#   Simple to document and to put on PATH once.
#
# Steps:
#   1. Build fsearch into ~/.local/bin/fsearch
#   2. If ~/.local/bin is not on PATH, add it once to ~/.bashrc
#
# Limit: editing ~/.bashrc does not change the *current* shell's PATH.
#   If we only updated the file, run:  source ~/.bashrc
#   (or open a new terminal)

set -euo pipefail

# Repo root = parent of scripts/
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PKG="${PKG:-./cmd/fsearch}"

BIN_DIR="${HOME}/.local/bin"
BIN_PATH="${BIN_DIR}/fsearch"
MARKER="fsearch: ~/.local/bin (added by make install)"
BASHRC="${HOME}/.bashrc"

# --- Step 1: build into ~/.local/bin --------------------------------------
mkdir -p "$BIN_DIR"
(cd "$ROOT" && go build -o "$BIN_PATH" "$PKG")

if [[ ! -x "$BIN_PATH" ]]; then
	echo "error: build did not produce $BIN_PATH" >&2
	exit 1
fi
echo "Installed: $BIN_PATH"

# --- Step 2: ensure ~/.local/bin is on PATH -------------------------------
path_has_bin_dir=false
case ":$PATH:" in
*":$BIN_DIR:"*) path_has_bin_dir=true ;;
esac

if $path_has_bin_dir; then
	echo "PATH already includes $BIN_DIR — fsearch should work in this shell."
	if found="$(command -v fsearch 2>/dev/null)" && [[ -n "$found" ]]; then
		echo "OK: fsearch is on PATH ($found)."
		if [[ "$found" != "$BIN_PATH" ]]; then
			echo "note: shell resolves fsearch to $found (not $BIN_PATH); check PATH order if that is unexpected."
		fi
	else
		echo "warning: $BIN_DIR is on PATH but fsearch was not found; try opening a new shell or: hash -r"
	fi
	exit 0
fi

# Not on PATH in this shell → persist for future shells (once).
already_configured=false
if [[ -f "$BASHRC" ]]; then
	if grep -qF "$MARKER" "$BASHRC" 2>/dev/null || grep -qF "$BIN_DIR" "$BASHRC" 2>/dev/null; then
		already_configured=true
	fi
fi

if $already_configured; then
	echo "PATH config for $BIN_DIR is already in $BASHRC"
else
	touch "$BASHRC"
	{
		echo ""
		echo "# $MARKER"
		echo "export PATH=\"$BIN_DIR:\$PATH\""
	} >>"$BASHRC"
	echo "Added $BIN_DIR to PATH in $BASHRC"
fi

echo ""
echo "This shell cannot see fsearch yet. Run:"
echo "  source $BASHRC"
echo "  # or open a new terminal"
echo "Then: fsearch --help"
