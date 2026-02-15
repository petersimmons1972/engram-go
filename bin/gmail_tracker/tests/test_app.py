"""Tests for main application logic"""

import pytest
from gmail_tracker.app import GmailJobTracker
from gmail_tracker.config import Config

def test_app_initializes():
    """Test that app initializes with config"""
    config = Config()
    app = GmailJobTracker(config)
    assert app is not None

def test_app_has_run_method():
    """Test that app has run method"""
    config = Config()
    app = GmailJobTracker(config)
    assert hasattr(app, 'run')

@pytest.mark.skip(reason="Requires full setup")
def test_app_runs():
    """Test that app runs successfully"""
    config = Config()
    app = GmailJobTracker(config)
    result = app.run()
    assert result == 0
