"""Email search functionality for Gmail API"""

from datetime import datetime, timedelta
from typing import List, Dict, Any, Optional

class EmailSearcher:
    """Searches Gmail for job application confirmation emails"""

    def __init__(self, gmail_service, config):
        """Initialize searcher

        Args:
            gmail_service: Authenticated Gmail API service
            config: Config object with search parameters
        """
        self.service = gmail_service
        self.config = config

    def build_search_query(self, after_date: datetime) -> str:
        """Build Gmail search query

        Args:
            after_date: Only include emails after this date

        Returns:
            Gmail search query string
        """
        # Format date for Gmail query
        date_str = after_date.strftime("%Y/%m/%d")

        # Build subject patterns OR clause
        subject_patterns = self.config.gmail['subject_patterns']
        subject_query = " OR ".join(f'"{pattern}"' for pattern in subject_patterns)

        # Build ATS domains OR clause
        ats_domains = self.config.gmail['known_ats_domains']
        domain_query = " OR ".join(f"{domain}" for domain in ats_domains)

        # Combine into full query
        query = f"after:{date_str} (subject:({subject_query}) OR from:({domain_query}))"

        return query

    def build_initial_search_query(self) -> str:
        """Build search query for initial run

        Returns:
            Search query for initial lookback period
        """
        lookback_days = self.config.gmail['initial_lookback_days']
        after_date = datetime.now() - timedelta(days=lookback_days)
        return self.build_search_query(after_date)

    def search_emails(self, query: str, max_results: int = None) -> List[Dict[str, Any]]:
        """Execute search and return email messages

        Args:
            query: Gmail search query
            max_results: Maximum emails to return

        Returns:
            List of email message dicts
        """
        if self.service is None:
            return []

        if max_results is None:
            max_results = self.config.gmail['max_emails_per_run']

        try:
            # Search for messages
            results = self.service.users().messages().list(
                userId='me',
                q=query,
                maxResults=max_results
            ).execute()

            messages = results.get('messages', []) if results else []

            # Fetch full message details
            full_messages = []
            for msg in messages:
                full_msg = self.service.users().messages().get(
                    userId='me',
                    id=msg['id'],
                    format='full'
                ).execute()
                full_messages.append(full_msg)

            return full_messages

        except Exception as e:
            raise RuntimeError(f"Failed to search emails: {e}")
