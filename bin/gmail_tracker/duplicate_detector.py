"""Duplicate detection using fuzzy matching"""

import re
import yaml
from pathlib import Path
from typing import Dict, List, Optional
from Levenshtein import ratio

# Common company suffixes to strip before matching
COMPANY_SUFFIXES = [
    r'\s+Inc\.?$',
    r'\s+LLC\.?$',
    r'\s+Corp\.?$',
    r'\s+Corporation$',
    r'\s+Labs$',
    r'\s+Co\.?$',
]

# Compiled regex for suffix stripping
SUFFIX_PATTERN = re.compile('|'.join(COMPANY_SUFFIXES), re.IGNORECASE)


def strip_company_suffix(name: str) -> str:
    """Strip common company suffixes from a name

    Args:
        name: Company name to process

    Returns:
        Name with common suffixes removed
    """
    return SUFFIX_PATTERN.sub('', name).strip()


class DuplicateDetector:
    """Detects duplicate companies using fuzzy matching"""

    def __init__(self, config):
        """Initialize detector

        Args:
            config: Config object
        """
        self.config = config
        self.threshold = config.duplicate_detection['fuzzy_match_threshold']
        self.aliases = self._load_aliases()

    def _load_aliases(self) -> Dict[str, List[str]]:
        """Load company aliases from YAML

        Returns:
            Dict mapping canonical name to variations
        """
        aliases_file = Path.home() / '.config' / 'gmail-job-tracker' / 'company_aliases.yaml'

        if not aliases_file.exists():
            return {}

        with open(aliases_file, 'r') as f:
            data = yaml.safe_load(f)

        # Handle empty or invalid YAML
        if data is None:
            data = {}

        # Build lookup dict
        alias_map = {}
        for entry in data.get('aliases', []):
            canonical = entry['canonical']
            for variation in entry.get('variations', []):
                alias_map[variation.lower()] = canonical

        return alias_map

    def fuzzy_match(self, name1: str, name2: str) -> float:
        """Calculate similarity between two company names

        Args:
            name1: First company name
            name2: Second company name

        Returns:
            Similarity score (0.0 to 1.0)
        """
        # Normalize names
        n1 = name1.lower().strip()
        n2 = name2.lower().strip()

        # Strip common suffixes before matching
        n1_stripped = strip_company_suffix(n1)
        n2_stripped = strip_company_suffix(n2)

        # Use Levenshtein ratio on stripped names
        return ratio(n1_stripped, n2_stripped)

    def check_alias(self, name: str) -> Optional[str]:
        """Check if name is an alias for a canonical company

        Args:
            name: Company name to check

        Returns:
            Canonical name if found, None otherwise
        """
        name_lower = name.lower()
        return self.aliases.get(name_lower)

    def find_best_match(self, name: str,
                        existing_companies: List[Dict]) -> Optional[Dict]:
        """Find best matching company from existing list

        Args:
            name: Company name to match
            existing_companies: List of existing company records

        Returns:
            Best matching company or None
        """
        # First check aliases
        canonical = self.check_alias(name)
        if canonical:
            for company in existing_companies:
                if company['name'].lower() == canonical.lower():
                    return company

        # Then fuzzy match
        best_match = None
        best_score = 0.0

        for company in existing_companies:
            score = self.fuzzy_match(name, company['name'])
            if score > best_score and score >= self.threshold:
                best_score = score
                best_match = company

        return best_match
