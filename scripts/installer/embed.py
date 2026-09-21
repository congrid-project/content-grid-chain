#!/usr/bin/env python3
"""Refresh the standalone installer's embedded state-sync helper."""
from pathlib import Path
root = Path(__file__).resolve().parents[2]
installer = root / 'cmd/congrid-site/downloads/install.sh'
text = installer.read_text()
a = text.index("<<'PY_STATE_SYNC'\n") + len("<<'PY_STATE_SYNC'\n")
b = text.index('\nPY_STATE_SYNC\n', a)
text = text[:a] + (root / 'scripts/installer/state_sync.py').read_text().rstrip() + text[b:]
installer.write_text(text)
