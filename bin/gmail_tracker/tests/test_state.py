"""Tests for state management"""

import pytest
from pathlib import Path
from datetime import datetime
from gmail_tracker.state import StateManager
import tempfile

def test_state_manager_initializes():
    """Test that state manager initializes"""
    with tempfile.NamedTemporaryFile(suffix='.json', delete=False) as f:
        state_file = Path(f.name)
    state_file.unlink()  # Delete it so StateManager creates it
    state = StateManager(state_file=state_file)
    assert state is not None
    state_file.unlink()  # Cleanup

def test_get_last_check_returns_none_initially():
    """Test that last check is None initially"""
    with tempfile.NamedTemporaryFile(suffix='.json', delete=False) as f:
        state_file = Path(f.name)
    state_file.unlink()  # Delete it so StateManager creates it
    state = StateManager(state_file=state_file)
    assert state.get_last_check() is None
    state_file.unlink()  # Cleanup

def test_update_last_check():
    """Test updating last check timestamp"""
    with tempfile.NamedTemporaryFile(suffix='.json', delete=False) as f:
        state_file = Path(f.name)
    state_file.unlink()  # Delete it so StateManager creates it
    state = StateManager(state_file=state_file)
    now = datetime.now()
    state.update_last_check(now)

    retrieved = state.get_last_check()
    assert retrieved is not None
    assert retrieved.date() == now.date()
    state_file.unlink()  # Cleanup
