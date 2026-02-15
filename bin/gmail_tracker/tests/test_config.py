"""Tests for configuration management"""

import pytest
from pathlib import Path
import tempfile
import yaml
from gmail_tracker.config import Config, ConfigError


def test_config_loads_from_yaml():
    """Test that config loads from YAML file and has required structure"""
    config = Config()
    # Check that required keys exist and have correct types
    assert 'host' in config.database
    assert 'port' in config.database
    assert isinstance(config.database['port'], int)
    assert 'check_interval_hours' in config.gmail
    assert isinstance(config.gmail['check_interval_hours'], int)


def test_config_has_ats_domains():
    """Test that known ATS domains are loaded"""
    config = Config()
    assert 'greenhouse.io' in config.gmail['known_ats_domains']
    assert 'lever.co' in config.gmail['known_ats_domains']


def test_config_loads_secrets():
    """Test that secrets are loaded from files"""
    config = Config()
    assert config.anthropic_api_key is not None
    assert config.db_password is not None


def test_config_missing_file():
    """Test that ConfigError is raised when config file doesn't exist"""
    nonexistent_path = Path("/tmp/nonexistent_config_12345.yaml")
    with pytest.raises(ConfigError, match="Configuration file not found"):
        Config(config_path=nonexistent_path)


def test_config_empty_file():
    """Test that ConfigError is raised when config file is empty"""
    with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
        config_path = Path(f.name)
        # Write nothing - empty file

    try:
        with pytest.raises(ConfigError, match="is empty"):
            Config(config_path=config_path)
    finally:
        config_path.unlink()


def test_config_malformed_yaml():
    """Test that ConfigError is raised when YAML is malformed"""
    with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
        config_path = Path(f.name)
        f.write("invalid: yaml: content:\n  - broken\n  - [unclosed")

    try:
        with pytest.raises(ConfigError, match="Failed to parse YAML"):
            Config(config_path=config_path)
    finally:
        config_path.unlink()


def test_config_missing_required_sections():
    """Test that ConfigError is raised when required sections are missing"""
    with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
        config_path = Path(f.name)
        # Only include database section, missing all others
        yaml.dump({'database': {'host': 'localhost', 'port': 5432}}, f)

    try:
        with pytest.raises(ConfigError, match="missing required sections"):
            Config(config_path=config_path)
    finally:
        config_path.unlink()


def test_config_missing_required_keys():
    """Test that ConfigError is raised when required keys are missing in a section"""
    config_data = {
        'database': {'host': 'localhost'},  # Missing port, database, user
        'gmail': {
            'check_interval_hours': 12,
            'initial_lookback_days': 7,
            'max_emails_per_run': 100,
            'known_ats_domains': [],
            'subject_patterns': []
        },
        'llm': {'provider': 'anthropic', 'model': 'claude-haiku-4', 'max_tokens': 1024, 'temperature': 0.0},
        'logging': {'level': 'INFO', 'rotate_after_days': 30, 'keep_error_emails_days': 90},
        'duplicate_detection': {'fuzzy_match_threshold': 0.85, 'position_reapply_window_days': 90}
    }

    with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
        config_path = Path(f.name)
        yaml.dump(config_data, f)

    try:
        with pytest.raises(ConfigError, match="Config validation failed"):
            Config(config_path=config_path)
    finally:
        config_path.unlink()


def test_config_missing_secrets_with_require_true():
    """Test that ConfigError is raised when secrets are missing and require_secrets=True"""
    # Create a valid config but without secret files
    config_data = {
        'database': {'host': 'localhost', 'port': 5432, 'database': 'test', 'user': 'test'},
        'gmail': {
            'check_interval_hours': 12,
            'initial_lookback_days': 7,
            'max_emails_per_run': 100,
            'known_ats_domains': ['greenhouse.io'],
            'subject_patterns': ['application received']
        },
        'llm': {'provider': 'anthropic', 'model': 'claude-haiku-4', 'max_tokens': 1024, 'temperature': 0.0},
        'logging': {'level': 'INFO', 'rotate_after_days': 30, 'keep_error_emails_days': 90},
        'duplicate_detection': {'fuzzy_match_threshold': 0.85, 'position_reapply_window_days': 90}
    }

    with tempfile.TemporaryDirectory() as tmpdir:
        config_path = Path(tmpdir) / 'config.yaml'
        with open(config_path, 'w') as f:
            yaml.dump(config_data, f)

        # Should raise error because secret files don't exist
        with pytest.raises(ConfigError, match="API key file not found"):
            Config(config_path=config_path, require_secrets=True)


def test_config_missing_secrets_with_require_false():
    """Test that Config loads successfully when secrets are missing and require_secrets=False"""
    # Create a valid config but without secret files
    config_data = {
        'database': {'host': 'localhost', 'port': 5432, 'database': 'test', 'user': 'test'},
        'gmail': {
            'check_interval_hours': 12,
            'initial_lookback_days': 7,
            'max_emails_per_run': 100,
            'known_ats_domains': ['greenhouse.io'],
            'subject_patterns': ['application received']
        },
        'llm': {'provider': 'anthropic', 'model': 'claude-haiku-4', 'max_tokens': 1024, 'temperature': 0.0},
        'logging': {'level': 'INFO', 'rotate_after_days': 30, 'keep_error_emails_days': 90},
        'duplicate_detection': {'fuzzy_match_threshold': 0.85, 'position_reapply_window_days': 90}
    }

    with tempfile.TemporaryDirectory() as tmpdir:
        config_path = Path(tmpdir) / 'config.yaml'
        with open(config_path, 'w') as f:
            yaml.dump(config_data, f)

        # Should load successfully but with None secrets
        config = Config(config_path=config_path, require_secrets=False)
        assert config.anthropic_api_key is None
        assert config.db_password is None
        assert config.database['host'] == 'localhost'


def test_config_invalid_section_type():
    """Test that ConfigError is raised when a section is not a dictionary"""
    config_data = {
        'database': "not a dictionary",  # Should be a dict
        'gmail': {
            'check_interval_hours': 12,
            'initial_lookback_days': 7,
            'max_emails_per_run': 100,
            'known_ats_domains': [],
            'subject_patterns': []
        },
        'llm': {'provider': 'anthropic', 'model': 'claude-haiku-4', 'max_tokens': 1024, 'temperature': 0.0},
        'logging': {'level': 'INFO', 'rotate_after_days': 30, 'keep_error_emails_days': 90},
        'duplicate_detection': {'fuzzy_match_threshold': 0.85, 'position_reapply_window_days': 90}
    }

    with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
        config_path = Path(f.name)
        yaml.dump(config_data, f)

    try:
        with pytest.raises(ConfigError, match="must be a dictionary"):
            Config(config_path=config_path)
    finally:
        config_path.unlink()
