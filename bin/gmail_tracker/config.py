"""Configuration management for Gmail Job Tracker"""

import yaml
from pathlib import Path
from typing import Dict, Any
import logging

logger = logging.getLogger(__name__)

class ConfigError(Exception):
    """Raised when configuration is invalid or missing"""
    pass

class Config:
    """Manages configuration loading and access"""

    REQUIRED_KEYS = {
        'database': ['host', 'port', 'database', 'user'],
        'gmail': ['check_interval_hours', 'initial_lookback_days', 'max_emails_per_run',
                  'known_ats_domains', 'subject_patterns'],
        'llm': ['provider', 'model', 'max_tokens', 'temperature'],
        'logging': ['level', 'rotate_after_days', 'keep_error_emails_days'],
        'duplicate_detection': ['fuzzy_match_threshold', 'position_reapply_window_days']
    }

    def __init__(self, config_path: Path = None, require_secrets: bool = True):
        """Initialize config from YAML and secret files

        Args:
            config_path: Path to config.yaml file. If None, uses default location.
            require_secrets: If True, raises error when secrets are missing.
                           If False, logs warning and continues.

        Raises:
            ConfigError: If config file doesn't exist, is malformed, or secrets are missing
        """
        if config_path is None:
            config_path = Path.home() / '.config' / 'gmail-job-tracker' / 'config.yaml'

        self._config_dir = config_path.parent
        self._require_secrets = require_secrets
        self._load_yaml(config_path)
        self._load_secrets()

    def _load_yaml(self, path: Path):
        """Load main YAML config

        Raises:
            ConfigError: If file doesn't exist, can't be parsed, or missing required keys
        """
        # Check if config file exists
        if not path.exists():
            raise ConfigError(
                f"Configuration file not found: {path}\n"
                f"Please create the config file at this location or specify a valid path."
            )

        # Try to load and parse YAML
        try:
            with open(path, 'r') as f:
                data = yaml.safe_load(f)
        except yaml.YAMLError as e:
            raise ConfigError(f"Failed to parse YAML config file {path}: {e}")
        except Exception as e:
            raise ConfigError(f"Failed to read config file {path}: {e}")

        # Check if data is valid
        if data is None:
            raise ConfigError(f"Config file {path} is empty")

        if not isinstance(data, dict):
            raise ConfigError(f"Config file {path} must contain a YAML dictionary")

        # Validate all required top-level keys exist
        missing_sections = []
        for section in self.REQUIRED_KEYS.keys():
            if section not in data:
                missing_sections.append(section)

        if missing_sections:
            raise ConfigError(
                f"Config file missing required sections: {', '.join(missing_sections)}\n"
                f"Required sections: {', '.join(self.REQUIRED_KEYS.keys())}"
            )

        # Validate required keys within each section
        validation_errors = []
        for section, required_keys in self.REQUIRED_KEYS.items():
            section_data = data[section]
            if not isinstance(section_data, dict):
                validation_errors.append(f"Section '{section}' must be a dictionary")
                continue

            for key in required_keys:
                if key not in section_data:
                    validation_errors.append(f"Missing required key '{key}' in section '{section}'")

        if validation_errors:
            raise ConfigError(
                f"Config validation failed:\n" + "\n".join(f"  - {err}" for err in validation_errors)
            )

        # All validation passed, load the config
        self.database = data['database']
        self.gmail = data['gmail']
        self.llm = data['llm']
        self.logging = data['logging']
        self.duplicate_detection = data['duplicate_detection']

    def _load_secrets(self):
        """Load secrets from separate files

        Raises:
            ConfigError: If require_secrets=True and any secret file is missing
        """
        api_key_file = self._config_dir / 'anthropic_api_key'
        db_pass_file = self._config_dir / 'db_password'

        self.anthropic_api_key = None
        self.db_password = None

        # Load API key
        if api_key_file.exists():
            try:
                self.anthropic_api_key = api_key_file.read_text().strip()
                if not self.anthropic_api_key:
                    logger.warning(f"API key file exists but is empty: {api_key_file}")
            except Exception as e:
                logger.error(f"Failed to read API key file {api_key_file}: {e}")
        else:
            msg = f"Anthropic API key file not found: {api_key_file}"
            if self._require_secrets:
                raise ConfigError(msg)
            else:
                logger.warning(msg)

        # Load database password
        if db_pass_file.exists():
            try:
                self.db_password = db_pass_file.read_text().strip()
                if not self.db_password:
                    logger.warning(f"Database password file exists but is empty: {db_pass_file}")
            except Exception as e:
                logger.error(f"Failed to read database password file {db_pass_file}: {e}")
        else:
            msg = f"Database password file not found: {db_pass_file}"
            if self._require_secrets:
                raise ConfigError(msg)
            else:
                logger.warning(msg)
