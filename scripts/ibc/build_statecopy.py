#!/usr/bin/env python3
"""Build the offline harness with the selected source tree's real DB adapter."""
import argparse
from pathlib import Path
import shutil
import subprocess
import tempfile

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--source', required=True, help='Exact production or candidate Go module root')
p.add_argument('--output', required=True)
a = p.parse_args()
source = Path(a.source).resolve()
output = Path(a.output).resolve()
harness = Path(__file__).resolve().parent / 'statecopy/main.go'
if not (source / 'go.mod').is_file():
    p.error('source is not a Go module root')
with tempfile.TemporaryDirectory(prefix='ibc-rehearsal-build-', dir=source) as d:
    stage = Path(d)
    shutil.copy2(harness, stage / 'main.go')
    # Reuse the selected release's database behavior byte for byte. In
    # particular, raw cosmos-db cannot distinguish some empty IAVL roots.
    shutil.copy2(source / 'cmd/content-grid-d/dbfix.go', stage / 'dbfix.go')
    subprocess.run(['go', 'build', '-tags', 'ibc_rehearsal', '-o', str(output),
                    './' + stage.name], cwd=source, check=True)
