"""Tests for logging functionality"""

import pytest
from pathlib import Path
from gmail_tracker.logger import Logger

def test_logger_initializes():
    """Test that logger initializes"""
    logger = Logger()
    assert logger is not None

def test_log_info():
    """Test logging info messages"""
    logger = Logger()
    logger.info("Test message")
    # Should not raise exception

def test_log_error():
    """Test logging error messages"""
    logger = Logger()
    logger.error("Error message")
    # Should not raise exception

def test_save_failed_email():
    """Test saving failed email to errors directory"""
    logger = Logger()
    email_id = "test123"
    content = "Test email content"
    reason = "Parse failure"

    path = logger.save_failed_email(email_id, content, reason)
    assert path.exists()

    # Cleanup
    path.unlink()
