#!/usr/bin/env python3
"""Run the standalone exporter offered for download on the website."""
from pathlib import Path
import runpy

runpy.run_path(str(Path(__file__).resolve().parents[1] / "backend/webassets/static/clone-lab.py"), run_name="__main__")
