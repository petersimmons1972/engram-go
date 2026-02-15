"""Pytest configuration for gmail_tracker tests"""

import sys
from pathlib import Path

# Add parent directory to Python path so tests can import gmail_tracker
parent_dir = Path(__file__).parent.parent.parent
if str(parent_dir) not in sys.path:
    sys.path.insert(0, str(parent_dir))
