"""State management for tracking last check time"""

import json
from pathlib import Path
from datetime import datetime
from typing import Optional

class StateManager:
    """Manages persistent state (last check timestamp)"""

    def __init__(self, state_file: Path = None):
        """Initialize state manager

        Args:
            state_file: Path to state JSON file
        """
        if state_file is None:
            state_file = Path.home() / '.config' / 'gmail-job-tracker' / 'state.json'

        self.state_file = state_file
        self._ensure_state_file()

    def _ensure_state_file(self):
        """Ensure state file exists"""
        if not self.state_file.exists():
            self.state_file.parent.mkdir(parents=True, exist_ok=True)
            self._write_state({'last_check': None})

    def _read_state(self) -> dict:
        """Read state from file"""
        with open(self.state_file, 'r') as f:
            return json.load(f)

    def _write_state(self, state: dict):
        """Write state to file"""
        with open(self.state_file, 'w') as f:
            json.dump(state, f, indent=2)

    def get_last_check(self) -> Optional[datetime]:
        """Get last check timestamp

        Returns:
            Last check datetime or None
        """
        state = self._read_state()
        last_check = state.get('last_check')

        if last_check:
            return datetime.fromisoformat(last_check)
        return None

    def update_last_check(self, timestamp: datetime):
        """Update last check timestamp

        Args:
            timestamp: New last check time
        """
        state = self._read_state()
        state['last_check'] = timestamp.isoformat()
        self._write_state(state)
