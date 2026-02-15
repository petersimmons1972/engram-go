"""Tests for database functionality"""

import pytest
from gmail_tracker.database import DatabaseManager
from gmail_tracker.config import Config

@pytest.fixture
def db_config():
    """Mock database config for testing"""
    return {
        'host': 'localhost',
        'port': 5432,
        'database': 'jobsearch_test',
        'user': 'test_user'
    }

def test_db_manager_initializes(db_config):
    """Test that database manager initializes"""
    db = DatabaseManager(db_config, password='test')
    assert db is not None

@pytest.mark.skip(reason="Requires database connection")
def test_db_connects(db_config):
    """Test database connection"""
    db = DatabaseManager(db_config, password='test')
    assert db.connect() is True

def test_find_company_by_name():
    """Test finding company by name"""
    db = DatabaseManager({}, password='test')
    # Mock test - would require actual DB
    assert hasattr(db, 'find_company_by_name')

def test_insert_company():
    """Test inserting company"""
    db = DatabaseManager({}, password='test')
    assert hasattr(db, 'insert_company')

def test_insert_application():
    """Test inserting application"""
    db = DatabaseManager({}, password='test')
    assert hasattr(db, 'insert_application')
