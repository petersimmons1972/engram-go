"""Logging functionality for Gmail Job Tracker"""

import logging
from pathlib import Path
from datetime import datetime
from typing import Optional

class Logger:
    """Manages logging and error file storage"""

    def __init__(self, log_level: str = "INFO"):
        """Initialize logger

        Args:
            log_level: Logging level (DEBUG, INFO, WARNING, ERROR)
        """
        self.log_dir = Path.home() / 'logs'
        self.error_dir = self.log_dir / 'gmail-job-tracker-errors'
        self.error_dir.mkdir(parents=True, exist_ok=True)

        # Set up logging
        log_file = self.log_dir / 'gmail-job-tracker.log'
        error_log_file = self.log_dir / 'gmail-job-tracker-errors.log'

        # Main logger
        self.logger = logging.getLogger('gmail_tracker')
        self.logger.setLevel(getattr(logging, log_level))

        # Console handler
        console_handler = logging.StreamHandler()
        console_handler.setLevel(getattr(logging, log_level))
        console_formatter = logging.Formatter(
            '[%(asctime)s] %(levelname)s: %(message)s',
            datefmt='%Y-%m-%d %H:%M:%S'
        )
        console_handler.setFormatter(console_formatter)
        self.logger.addHandler(console_handler)

        # File handler
        file_handler = logging.FileHandler(log_file)
        file_handler.setLevel(getattr(logging, log_level))
        file_handler.setFormatter(console_formatter)
        self.logger.addHandler(file_handler)

        # Error file handler
        error_handler = logging.FileHandler(error_log_file)
        error_handler.setLevel(logging.ERROR)
        error_handler.setFormatter(console_formatter)
        self.logger.addHandler(error_handler)

    def info(self, message: str):
        """Log info message"""
        self.logger.info(message)

    def warning(self, message: str):
        """Log warning message"""
        self.logger.warning(message)

    def error(self, message: str):
        """Log error message"""
        self.logger.error(message)

    def debug(self, message: str):
        """Log debug message"""
        self.logger.debug(message)

    def success(self, message: str):
        """Log success message"""
        self.logger.info(f"SUCCESS: {message}")

    def duplicate(self, message: str):
        """Log duplicate detection"""
        self.logger.info(f"DUPLICATE: {message}")

    def partial(self, message: str):
        """Log partial extraction"""
        self.logger.warning(f"PARTIAL: {message}")

    def llm_fallback(self, message: str):
        """Log LLM fallback usage"""
        self.logger.info(f"LLM_FALLBACK: {message}")

    def save_failed_email(self, email_id: str, content: str,
                         reason: str) -> Path:
        """Save failed email to errors directory

        Args:
            email_id: Gmail message ID
            content: Email content
            reason: Failure reason

        Returns:
            Path to saved file
        """
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        filename = f"{timestamp}_{email_id}.txt"
        filepath = self.error_dir / filename

        with open(filepath, 'w') as f:
            f.write(f"===== EMAIL METADATA =====\n")
            f.write(f"Email ID: {email_id}\n")
            f.write(f"Timestamp: {timestamp}\n")
            f.write(f"Reason: {reason}\n\n")
            f.write(f"===== EMAIL CONTENT =====\n")
            f.write(content)

        self.error(f"Failed email saved to {filepath}")
        return filepath
