"""Database operations for job application tracking"""

import psycopg2
from psycopg2.extras import RealDictCursor
from typing import Optional, Dict, Any
from datetime import datetime

class DatabaseManager:
    """Manages PostgreSQL database operations"""

    def __init__(self, db_config: Dict[str, Any], password: str):
        """Initialize database manager

        Args:
            db_config: Database configuration dict
            password: Database password
        """
        self.config = db_config
        self.password = password
        self.conn = None
        self.cursor = None

    def connect(self) -> bool:
        """Connect to PostgreSQL database

        Returns:
            True if connected successfully
        """
        try:
            self.conn = psycopg2.connect(
                host=self.config.get('host', 'localhost'),
                port=self.config.get('port', 5432),
                database=self.config['database'],
                user=self.config['user'],
                password=self.password,
                cursor_factory=RealDictCursor
            )
            self.cursor = self.conn.cursor()
            return True
        except Exception as e:
            raise RuntimeError(f"Failed to connect to database: {e}")

    def disconnect(self):
        """Close database connection"""
        if self.cursor:
            self.cursor.close()
        if self.conn:
            self.conn.close()

    def find_company_by_name(self, name: str) -> Optional[Dict[str, Any]]:
        """Find company by name (case-insensitive)

        Args:
            name: Company name

        Returns:
            Company record or None
        """
        query = "SELECT * FROM companies WHERE LOWER(name) = LOWER(%s)"
        self.cursor.execute(query, (name,))
        return self.cursor.fetchone()

    def insert_company(self, name: str, website: str = None,
                      description: str = None) -> int:
        """Insert new company record

        Args:
            name: Company name
            website: Company website
            description: Company description

        Returns:
            Company ID
        """
        query = """
            INSERT INTO companies (name, website, description, created_at, updated_at)
            VALUES (%s, %s, %s, NOW(), NOW())
            RETURNING id
        """
        self.cursor.execute(query, (name, website, description))
        self.conn.commit()
        return self.cursor.fetchone()['id']

    def find_duplicate_application(self, company_id: int,
                                   position_title: str,
                                   within_days: int = 90) -> Optional[Dict[str, Any]]:
        """Check for duplicate application

        Args:
            company_id: Company ID
            position_title: Position title
            within_days: Check within this many days

        Returns:
            Existing application or None
        """
        query = """
            SELECT * FROM applications
            WHERE company_id = %s
            AND LOWER(position_title) = LOWER(%s)
            AND application_date >= (CURRENT_DATE - INTERVAL '%s days')
        """
        self.cursor.execute(query, (company_id, position_title, within_days))
        return self.cursor.fetchone()

    def insert_application(self, company_id: int, position_title: str,
                          application_date: datetime = None,
                          job_url: str = None, notes: str = None) -> int:
        """Insert new application record

        Args:
            company_id: Company ID
            position_title: Position title
            application_date: Date applied
            job_url: Job posting URL
            notes: Additional notes

        Returns:
            Application ID
        """
        if application_date is None:
            application_date = datetime.now().date()

        query = """
            INSERT INTO applications (
                company_id, position_title, application_date,
                status, job_url, notes, created_at, updated_at
            ) VALUES (%s, %s, %s, 'applied', %s, %s, NOW(), NOW())
            RETURNING id
        """
        self.cursor.execute(query, (
            company_id, position_title, application_date,
            job_url, notes
        ))
        self.conn.commit()
        return self.cursor.fetchone()['id']
