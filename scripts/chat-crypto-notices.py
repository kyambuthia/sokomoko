#!/usr/bin/env python3
"""Bundle dependency license texts alongside the browser WASM distribution."""
import json
from pathlib import Path
import subprocess
import sys

root = Path(__file__).resolve().parent.parent
metadata = json.loads(subprocess.check_output([
    'cargo', 'metadata', '--manifest-path', str(root / 'web/crypto/Cargo.toml'),
    '--locked', '--format-version', '1', '--filter-platform', 'wasm32-unknown-unknown'
]))
parts = ['Third-party notices for the pinned Sokomoko MLS browser build.\n'
         'This inventory includes Cargo-resolved optional dependencies as well.\n']
for package in sorted(metadata['packages'], key=lambda p: (p['name'], p['version'])):
    if package['name'] == 'sokomoko-mls':
        continue
    directory = Path(package['manifest_path']).parent
    parts.append(f"\n{'=' * 72}\n{package['name']} {package['version']}\n"
                 f"License: {package.get('license') or 'See package license file'}\n"
                 f"Source: {package.get('repository') or package['source']}\n")
    files = sorted(p for p in directory.iterdir() if p.is_file() and
                   (p.name.upper().startswith(('LICENSE', 'LICENCE', 'COPYING', 'NOTICE'))))
    for path in files:
        parts.append(f'\n--- {path.name} ---\n{path.read_text(errors="replace")}\n')
output = Path(sys.argv[1]) if len(sys.argv) > 1 else root / 'internal/ui/static/crypto'
output.mkdir(parents=True, exist_ok=True)
(output / 'THIRD_PARTY_NOTICES.txt').write_text(''.join(parts))
