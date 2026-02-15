"""Tests for email search functionality"""

import pytest
from datetime import datetime, timedelta
from gmail_tracker.email_searcher import EmailSearcher
from gmail_tracker.config import Config

def test_searcher_initializes():
    """Test that searcher initializes with config"""
    config = Config()
    searcher = EmailSearcher(None, config)  # None service for testing
    assert searcher is not None

def test_build_search_query_with_date():
    """Test that search query includes date filter"""
    config = Config()
    searcher = EmailSearcher(None, config)

    after_date = datetime(2026, 1, 15)
    query = searcher.build_search_query(after_date)

    assert "after:2026/01/15" in query
    assert "greenhouse.io" in query
    assert "application received" in query

def test_build_initial_search_query():
    """Test initial search query (7 days lookback)"""
    config = Config()
    searcher = EmailSearcher(None, config)

    query = searcher.build_initial_search_query()

    # Should include date 7 days ago
    assert "after:" in query
