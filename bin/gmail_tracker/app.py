"""Main application logic for Gmail Job Tracker"""

from datetime import datetime, timedelta
from typing import Dict, List, Optional

from .config import Config
from .gmail_auth import GmailAuthenticator
from .email_searcher import EmailSearcher
from .email_parser import EmailParser
from .llm_extractor import LLMExtractor
from .database import DatabaseManager
from .duplicate_detector import DuplicateDetector
from .state import StateManager
from .logger import Logger

class GmailJobTracker:
    """Main application for tracking job applications from Gmail"""

    def __init__(self, config: Config):
        """Initialize application

        Args:
            config: Configuration object
        """
        self.config = config
        self.logger = Logger(config.logging['level'])
        self.state = StateManager()

        # Initialize components (will connect during run)
        self.gmail_auth = None
        self.gmail_service = None
        self.email_searcher = None
        self.email_parser = None
        self.llm_extractor = None
        self.database = None
        self.duplicate_detector = None

        # Statistics
        self.stats = {
            'emails_processed': 0,
            'applications_inserted': 0,
            'duplicates_found': 0,
            'needs_review': 0,
            'errors': 0
        }

    def run(self, dry_run: bool = False) -> int:
        """Run the job tracker

        Args:
            dry_run: If True, don't insert to database

        Returns:
            Exit code (0 = success)
        """
        print("DEBUG: run() method called")
        try:
            print("DEBUG: About to log starting message")
            self.logger.info(f"Starting Gmail Job Tracker v1.0.0")
            print("DEBUG: Logger message sent")

            # Initialize components
            print("DEBUG: Initializing components...")
            self._initialize_components()
            print("DEBUG: Components initialized successfully")

            # Determine search timeframe
            last_check = self.state.get_last_check()
            if last_check:
                self.logger.info(f"Searching emails since {last_check}")
                query = self.email_searcher.build_search_query(last_check)
            else:
                self.logger.info(f"Initial run: searching last {self.config.gmail['initial_lookback_days']} days")
                query = self.email_searcher.build_initial_search_query()

            # Search for emails
            print(f"DEBUG: Searching with query: {query}")
            emails = self.email_searcher.search_emails(query)
            print(f"DEBUG: Raw emails returned: {len(emails) if emails else 'None'}")
            self.logger.info(f"Found {len(emails)} emails to process")

            # Filter out None emails
            emails = [email for email in emails if email is not None]
            print(f"DEBUG: After filtering None emails: {len(emails)}")

            # Process each email
            print(f"DEBUG: Processing {len(emails)} emails")
            for i, email in enumerate(emails):
                print(f"DEBUG: Processing email {i+1}/{len(emails)} - ID: {email.get('id', 'None') if email else 'None'}")
                if email is None:
                    self.logger.warning("Skipping None email")
                    continue
                self._process_email(email, dry_run)

            # Update last check timestamp
            if not dry_run and len(emails) > 0:
                self.state.update_last_check(datetime.now())
                self.logger.info("Updated last check timestamp")

            # Log summary
            self._log_summary()

            return 0

        except Exception as e:
            import traceback
            self.logger.error(f"Application error: {e}")
            self.logger.error(f"Traceback: {traceback.format_exc()}")
            return 1
        finally:
            self._cleanup()

    def _initialize_components(self):
        """Initialize all components"""
        print("DEBUG: Starting Gmail authentication...")
        # Gmail authentication
        self.gmail_auth = GmailAuthenticator()
        print("DEBUG: Created GmailAuthenticator")
        self.gmail_service = self.gmail_auth.authenticate(interactive=False)
        print("DEBUG: Authenticated with Gmail API")
        self.logger.info("Authenticated with Gmail API")

        print("DEBUG: Creating email components...")
        # Email components
        self.email_searcher = EmailSearcher(self.gmail_service, self.config)
        print("DEBUG: Created EmailSearcher")
        self.email_parser = EmailParser(self.config)
        print("DEBUG: Created EmailParser")

        print("DEBUG: Creating LLM extractor...")
        # LLM extractor
        self.llm_extractor = LLMExtractor(self.config, self.config.anthropic_api_key)
        print("DEBUG: Created LLMExtractor")

        print("DEBUG: Connecting to database...")
        # Database
        self.database = DatabaseManager(self.config.database, self.config.db_password)
        print("DEBUG: Created DatabaseManager")
        self.database.connect()
        print("DEBUG: Connected to database")
        self.logger.info("Connected to database")

        print("DEBUG: Creating duplicate detector...")
        # Duplicate detector
        self.duplicate_detector = DuplicateDetector(self.config)
        print("DEBUG: Created DuplicateDetector")

    def _process_email(self, email: Dict, dry_run: bool):
        """Process single email

        Args:
            email: Gmail message dict
            dry_run: If True, don't insert to database
        """
        try:
            self.stats['emails_processed'] += 1
            email_id = email['id']

            # Parse email with pattern matching
            extracted = self.email_parser.parse_email(email)

            # Handle parsing errors
            if extracted is None:
                self.logger.error(f"Failed to parse email {email_id}: parse_email returned None")
                return

            # Check if extraction is complete
            needs_llm = not extracted.get('company') or not extracted.get('position')

            # Use LLM fallback if needed
            if needs_llm:
                self.logger.llm_fallback(f"Email {email_id}: pattern match incomplete")
                body = self.email_parser.extract_email_body(email)
                llm_result = self.llm_extractor.extract(body)

                # Merge results (prefer pattern match, fallback to LLM)
                extracted['company'] = extracted.get('company') or llm_result.get('company')
                extracted['position'] = extracted.get('position') or llm_result.get('position')
                extracted['application_id'] = extracted.get('application_id') or llm_result.get('application_id')
                extracted['job_url'] = extracted.get('job_url') or llm_result.get('job_url')

            # Check if still incomplete
            if not extracted.get('company') or not extracted.get('position'):
                self._handle_incomplete_extraction(email, extracted)
                return

            # Log extraction
            self.logger.success(f"Extracted - {extracted.get('company', 'Unknown')}, {extracted.get('position', 'Unknown')}")

            if dry_run:
                self.logger.info("[DRY RUN] Would insert to database")
                return

            # Insert to database
            self._insert_application(extracted)

        except Exception as e:
            self.logger.error(f"Failed to process email {email.get('id', 'unknown')}: {e}")
            self.stats['errors'] += 1

    def _handle_incomplete_extraction(self, email: Dict, extracted: Dict):
        """Handle incomplete extraction

        Args:
            email: Gmail message dict
            extracted: Partially extracted data
        """
        email_id = email['id']
        missing = []
        if not extracted['company']:
            missing.append('company')
        if not extracted['position']:
            missing.append('position')

        self.logger.partial(f"Email {email_id} - missing: {', '.join(missing)}")

        # Save email for manual review
        body = self.email_parser.extract_email_body(email)
        reason = f"Missing fields: {', '.join(missing)}"
        self.logger.save_failed_email(email_id, body, reason)

        self.stats['needs_review'] += 1

    def _insert_application(self, extracted: Dict):
        """Insert application to database

        Args:
            extracted: Extracted data dict
        """
        # Find or create company
        company = self.database.find_company_by_name(extracted['company'])

        if not company:
            # Try fuzzy matching
            # (Would need to fetch all companies - simplified here)
            company_id = self.database.insert_company(
                name=extracted['company'],
                website=None,
                description=None
            )
        else:
            company_id = company['id']

        # Check for duplicate application
        duplicate = self.database.find_duplicate_application(
            company_id,
            extracted['position'],
            within_days=self.config.duplicate_detection['position_reapply_window_days']
        )

        if duplicate:
            self.logger.duplicate(
                f"{extracted['company']}, {extracted['position']} "
                f"(application #{duplicate['id']} from {duplicate['application_date']})"
            )
            self.stats['duplicates_found'] += 1
            return

        # Insert new application
        app_id = self.database.insert_application(
            company_id=company_id,
            position_title=extracted['position'],
            application_date=datetime.now().date(),
            job_url=extracted.get('job_url'),
            notes=f"Application ID: {extracted.get('application_id')}" if extracted.get('application_id') else None
        )

        self.logger.success(f"Inserted application #{app_id} (company_id: {company_id})")
        self.stats['applications_inserted'] += 1

    def _log_summary(self):
        """Log run summary"""
        self.logger.info(
            f"Processed {self.stats['emails_processed']} emails: "
            f"{self.stats['applications_inserted']} inserted, "
            f"{self.stats['duplicates_found']} duplicates, "
            f"{self.stats['needs_review']} need review"
        )

        if self.stats['errors'] > 0:
            self.logger.warning(f"{self.stats['errors']} errors occurred")

    def _cleanup(self):
        """Cleanup resources"""
        if self.database:
            self.database.disconnect()
