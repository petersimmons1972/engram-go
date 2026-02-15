"""Email parsing and data extraction"""

import re
import base64
from typing import Dict, Optional, Any, List
from email import message_from_bytes

class EmailParser:
    """Parses job application confirmation emails"""

    def __init__(self, config):
        """Initialize parser

        Args:
            config: Config object with extraction patterns
        """
        self.config = config
        self._load_patterns()
        self._load_ats_patterns()

    # Suffixes to strip from company names for normalization
    COMPANY_SUFFIXES = [
        r',?\s+Inc\.?$',
        r',?\s+LLC\.?$',
        r',?\s+Corp\.?$',
        r',?\s+Corporation$',
        r',?\s+Ltd\.?$',
        r',?\s+Limited$',
    ]

    # Known ATS platform domains for detection
    KNOWN_ATS_DOMAINS = {
        'greenhouse.io': 'greenhouse',
        'lever.co': 'lever',
        'myworkday.com': 'workday',
        'myworkdayjobs.com': 'workday',
        'wd1.myworkdaysite.com': 'workday',
        'wd3.myworkdaysite.com': 'workday',
        'wd5.myworkdaysite.com': 'workday',
        'icims.com': 'icims',
        'taleo.net': 'taleo',
        'smartrecruiters.com': 'smartrecruiters',
        'ashbyhq.com': 'ashby',
        'bamboohr.com': 'bamboohr',
        'jazzhr.com': 'jazzhr',
        'applytojob.com': 'jazzhr',  # JazzHR alias
        'jobvite.com': 'jobvite',
        'workable.com': 'workable',
        'breezy.hr': 'breezy',
        'recruitee.com': 'recruitee',
        'fountain.com': 'fountain',
        'recruiterbox.com': 'recruiterbox',
    }

    def _load_patterns(self):
        """Load regex patterns from config"""
        # These would normally come from patterns.yaml
        # For now, hardcode common patterns
        self.company_patterns = [
            r"Thank you for applying to ([A-Z][a-zA-Z\s&.]+?)(?:\.|for|to)",
            r"application (?:to|at) ([A-Z][a-zA-Z\s&]+?)(?:\.|,|$|\s+for)",
            # Lever-style and generic patterns
            r"interest in ([A-Z][a-zA-Z\s&.]+?)!",
            r"team at ([A-Z][a-zA-Z\s&]+?)(?:\s+will|\s+is|\s+has|\s+would|,|\.|$)",
            r"(?<!application )at ([A-Z][a-zA-Z\s&]+?)\.",
            r"from ([A-Z][a-zA-Z\s&.]+?) (?:is|has|would)",
        ]

        self.position_patterns = [
            r"Position:\s*(.+?)(?:\n|$)",
            r"(?:for|to) the position of (.+?)(?:\.|\n)",
            r"You applied for:?\s*(.+?)(?:\n|$)",
            # Additional patterns for Lever-style and generic emails
            r"application for ([^.]+?)(?:\.|,| at|\n)",
            r"applying (?:for|to) ([^.]+?)(?:\.|\n)",
            r"role (?:of|as) ([^.]+?)(?:\.|\n)",
            r"interested in (?:the )?([^.]+?) (?:position|role)",
        ]

        self.app_id_patterns = [
            r"Application ID:\s*([A-Z0-9-]+)",
            r"Reference(?:\s+number)?:\s*([A-Z0-9-]+)",
            r"Confirmation(?:\s+code)?:\s*([A-Z0-9-]+)"
        ]

    def _load_ats_patterns(self):
        """Load ATS-specific extraction patterns"""
        self.ats_patterns = {
            'workday': {
                'subject_patterns': [
                    r"Your application (?:has been received|was received|to .+)",
                    r"Application Received",
                    r"Thank you for applying",
                    r"Application Confirmation",
                    r"(?:We received|Successfully submitted) your application",
                ],
                'company_patterns': [
                    r"Thank you for (?:your )?interest in ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"application (?:to|at|with) ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,| has)",
                    r"Thank you for applying to ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"([A-Z][a-zA-Z0-9\s&.'-]+?) Career[s]? Site",
                    r"([A-Z][a-zA-Z0-9\s&.'-]+?) - Workday",
                ],
                'position_patterns': [
                    r"Position:\s*(.+?)(?:\n|$)",
                    r"Job Title:\s*(.+?)(?:\n|$)",
                    r"Role:\s*(.+?)(?:\n|$)",
                    r"application for ([A-Z][a-zA-Z0-9\s]+?) has been",
                    r"applied for (?:the )?([A-Z][a-zA-Z0-9\s]+?)(?:\.|,| at| -|\n)",
                    r"application for (?:the )?([A-Z][a-zA-Z0-9\s]+?)(?:\.|,| at| -|\n)",
                    r"interest in (?:the )?([A-Z][a-zA-Z0-9\s]+?) (?:position|role|opportunity)",
                ],
                'app_id_patterns': [
                    r"Job Requisition(?:\s+ID)?:\s*([A-Z0-9-]+)",
                    r"Requisition(?:\s+ID)?:\s*([A-Z0-9-]+)",
                    r"Application(?:\s+ID)?:\s*([A-Z0-9-]+)",
                    r"Req(?:\s+)?ID:\s*([A-Z0-9-]+)",
                    r"R(?:eq)?-?(\d{5,})",
                ],
            },
            'icims': {
                'subject_patterns': [
                    r"Application Confirmation",
                    r"Thank you for your application",
                    r"Your application has been received",
                    r"Application Submitted",
                ],
                'company_patterns': [
                    r"applied (?:to|at|for a position at) ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"Thank you for applying to ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"interest in (?:joining )?([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"at ([A-Z][a-zA-Z0-9\s&.'-]+?)\s+(?:is|has|we)",
                ],
                'position_patterns': [
                    r"Position:\s*(.+?)(?:\n|$)",
                    r"Job:\s*(.+?)(?:\n|$)",
                    r"applied for (?:the )?(.+?)(?:\.|,|\n)",
                    r"application for (?:the )?([^.]+?) (?:position|role|at)",
                ],
                'app_id_patterns': [
                    r"Job(?:\s+)?ID:\s*(\d+)",
                    r"Reference(?:\s+)?(?:Number|#|ID)?:\s*([A-Z0-9-]+)",
                    r"iCIMS(?:\s+)?ID:\s*(\d+)",
                ],
            },
            'taleo': {
                'subject_patterns': [
                    r"Application (?:Received|Confirmation|Submitted)",
                    r"Thank you for applying",
                    r"Your application to",
                    r"Confirmation of Application",
                ],
                'company_patterns': [
                    r"applied (?:to|at|with) ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,| for)",
                    r"Thank you for (?:your )?interest in ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"([A-Z][a-zA-Z0-9\s&.'-]+?) (?:Careers|Jobs|Recruiting)",
                    r"application (?:to|at) ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                ],
                'position_patterns': [
                    r"Job Title:\s*(.+?)(?:\n|$)",
                    r"Position:\s*(.+?)(?:\n|$)",
                    r"applied for:?\s*(.+?)(?:\n|$)",
                    r"Role:\s*(.+?)(?:\n|$)",
                ],
                'app_id_patterns': [
                    r"Requisition(?:\s+)?(?:Number|#|ID)?:\s*([A-Z0-9-]+)",
                    r"Job(?:\s+)?(?:Number|#|ID)?:\s*([A-Z0-9-]+)",
                    r"Application(?:\s+)?(?:Number|#|ID)?:\s*([A-Z0-9-]+)",
                ],
            },
            'smartrecruiters': {
                'subject_patterns': [
                    r"Application received",
                    r"Thank you for your application",
                    r"Your application to",
                    r"We received your application",
                ],
                'company_patterns': [
                    r"Thank you for applying to ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"application (?:to|at) ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"([A-Z][a-zA-Z0-9\s&.'-]+?) via SmartRecruiters",
                    r"interest in (?:joining )?([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                ],
                'position_patterns': [
                    r"for (?:the )?position[:]?\s*(.+?)(?:\.|,|\n)",
                    r"applied for[:]?\s*(.+?)(?:\.|,|\n| at)",
                    r"Role:\s*(.+?)(?:\n|$)",
                    r"Position:\s*(.+?)(?:\n|$)",
                ],
                'app_id_patterns': [
                    r"Application(?:\s+)?ID:\s*([a-f0-9-]+)",
                    r"Reference:\s*([A-Z0-9-]+)",
                ],
            },
            'ashby': {
                'subject_patterns': [
                    r"Application received",
                    r"Thank you for applying",
                    r"We received your application",
                    r"Application confirmation",
                ],
                'company_patterns': [
                    r"Thank you for applying to ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"application (?:to|at) ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"interest in (?:joining )?([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"team at ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:\s+will|\s+has|\.|,)",
                ],
                'position_patterns': [
                    r"Position:\s*(.+?)(?:\n|$)",
                    r"application for (?:the )?([A-Z][a-zA-Z0-9\s]+?) role",
                    r"for (?:the )?([A-Z][a-zA-Z0-9\s]+?) role",
                    r"applied for (?:the )?([A-Z][a-zA-Z0-9\s]+?)(?:\.|,|\n| at)",
                ],
                'app_id_patterns': [
                    r"Application(?:\s+)?ID:\s*([a-zA-Z0-9-]+)",
                    r"Reference:\s*([A-Z0-9-]+)",
                ],
            },
            'bamboohr': {
                'subject_patterns': [
                    r"Application Received",
                    r"Thank you for applying",
                    r"Your application has been submitted",
                    r"Application Confirmation",
                ],
                'company_patterns': [
                    r"Thank you for applying to ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"application (?:to|at|with) ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"interest in ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"([A-Z][a-zA-Z0-9\s&.'-]+?) via BambooHR",
                ],
                'position_patterns': [
                    r"Position:\s*(.+?)(?:\n|$)",
                    r"Job:\s*(.+?)(?:\n|$)",
                    r"for (?:the )?([A-Z][a-zA-Z0-9\s]+?) position",
                    r"applied for (?:the )?([A-Z][a-zA-Z0-9\s]+?)(?:\.|,|\n| at)",
                ],
                'app_id_patterns': [
                    r"Application(?:\s+)?ID:\s*(\d+)",
                    r"Reference(?:\s+)?#?:\s*([A-Z0-9-]+)",
                ],
            },
            'jazzhr': {
                'subject_patterns': [
                    r"Application Received",
                    r"Thank you for your application",
                    r"Your application has been received",
                    r"Application Submitted Successfully",
                ],
                'company_patterns': [
                    r"Thank you for applying to ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"application (?:to|at|for a position at) ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"interest in (?:joining )?([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                ],
                'position_patterns': [
                    r"Position:\s*(.+?)(?:\n|$)",
                    r"Job:\s*(.+?)(?:\n|$)",
                    r"for (?:the )?([A-Z][a-zA-Z0-9\s]+?) opening",
                    r"applied for (?:the )?([A-Z][a-zA-Z0-9\s]+?)(?:\.|,|\n| at)",
                ],
                'app_id_patterns': [
                    r"Job(?:\s+)?ID:\s*([A-Za-z0-9-]+)",
                    r"Application(?:\s+)?ID:\s*([A-Za-z0-9-]+)",
                ],
            },
            'jobvite': {
                'subject_patterns': [
                    r"Application Confirmation",
                    r"Thank you for applying",
                    r"Your application has been received",
                    r"Application Received",
                ],
                'company_patterns': [
                    r"Thank you for applying to ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"application (?:to|at) ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"interest in (?:joining )?([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"([A-Z][a-zA-Z0-9\s&.'-]+?) Careers",
                ],
                'position_patterns': [
                    r"Position:\s*(.+?)(?:\n|$)",
                    r"Job:\s*(.+?)(?:\n|$)",
                    r"applied for (?:the )?(.+?)(?:\.|,|\n| at)",
                    r"for (?:the )?([^.]+?) (?:position|role)",
                ],
                'app_id_patterns': [
                    r"Requisition(?:\s+)?ID:\s*([A-Z0-9-]+)",
                    r"Job(?:\s+)?ID:\s*([A-Z0-9-]+)",
                    r"Application(?:\s+)?ID:\s*([A-Za-z0-9-]+)",
                ],
            },
            'greenhouse': {
                'subject_patterns': [
                    r"Application received",
                    r"Thank you for applying",
                    r"Your application to",
                    r"Application confirmation",
                ],
                'company_patterns': [
                    r"Thank you for applying to ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"application (?:to|at) ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"interest in ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"team at ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:\s+will|\s+has|\.|,)",
                ],
                'position_patterns': [
                    r"Position:\s*(.+?)(?:\n|$)",
                    r"Role:\s*(.+?)(?:\n|$)",
                    r"Applied for:\s*(.+?)(?:\n|$)",
                    r"application for (?:the )?([A-Z][a-zA-Z0-9\s]+?)(?:\.|,|\n| at)",
                    r"for (?:the )?([A-Z][a-zA-Z0-9\s]+?) (?:position|role)",
                    r"applied for (?:the )?([A-Z][a-zA-Z0-9\s]+?)(?:\.|,|\n| at)",
                ],
                'app_id_patterns': [
                    r"Application(?:\s+)?ID:\s*(\d+)",
                    r"Reference:\s*([A-Z0-9-]+)",
                ],
            },
            'lever': {
                'subject_patterns': [
                    r"Thank you for applying",
                    r"Application received",
                    r"Your application to",
                    r"Application confirmation",
                ],
                'company_patterns': [
                    r"interest in ([A-Z][a-zA-Z0-9\s&.'-]+?)!",
                    r"Thank you for applying to ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"application (?:to|at) ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:!|\.|,)",
                    r"team at ([A-Z][a-zA-Z0-9\s&.'-]+?)(?:\s+will|\s+has|\.|,)",
                ],
                'position_patterns': [
                    r"Position:\s*(.+?)(?:\n|$)",
                    r"for (?:the )?([A-Z][a-zA-Z0-9\s]+?) (?:position|role)",
                    r"applied for (?:the )?([A-Z][a-zA-Z0-9\s]+?)(?:\.|,|\n| position| role| at)",
                ],
                'app_id_patterns': [
                    r"Application(?:\s+)?ID:\s*([a-f0-9-]+)",
                    r"Reference:\s*([A-Z0-9-]+)",
                ],
            },
        }

    def extract_company_from_headers(self, headers: list) -> Optional[str]:
        """Extract company name from email headers

        Args:
            headers: List of email header dicts

        Returns:
            Company name or None
        """
        for header in headers:
            if header['name'] == 'From':
                # Extract name before email address
                match = re.search(r'(.+?)\s*<', header['value'])
                if match:
                    name = match.group(1).strip()
                    # Remove common recruiting suffixes
                    name = re.sub(r'\s+(Recruiting|Talent|Team|via.*)$', '', name)
                    return self.normalize_company_name(name)
        return None

    def normalize_company_name(self, name: str) -> str:
        """Normalize company name by stripping common suffixes

        Args:
            name: Raw company name

        Returns:
            Normalized company name
        """
        if not name:
            return name

        result = name.strip()
        for suffix_pattern in self.COMPANY_SUFFIXES:
            result = re.sub(suffix_pattern, '', result, flags=re.IGNORECASE)

        return result.strip()

    def detect_ats_platform(self, headers: List[Dict[str, str]], body: str) -> Optional[str]:
        """Detect which ATS platform sent the email

        Args:
            headers: List of email header dicts
            body: Email body text

        Returns:
            ATS platform name ('greenhouse', 'lever', 'workday', etc.) or None
        """
        # First, check sender domain
        for header in headers:
            if header['name'] == 'From':
                email_match = re.search(r'<([^>]+)>|([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+)', header['value'])
                if email_match:
                    email = email_match.group(1) or email_match.group(2)
                    domain_match = re.search(r'@([a-zA-Z0-9.-]+)', email)
                    if domain_match:
                        domain = domain_match.group(1).lower()
                        # Check for exact match
                        if domain in self.KNOWN_ATS_DOMAINS:
                            return self.KNOWN_ATS_DOMAINS[domain]
                        # Check for subdomain match
                        for known_domain, ats_name in self.KNOWN_ATS_DOMAINS.items():
                            if domain.endswith('.' + known_domain) or domain == known_domain:
                                return ats_name

        # Check email headers for ATS-specific headers
        for header in headers:
            header_name = header['name'].lower()
            header_value = header['value'].lower()

            # Check for ATS-specific headers
            if 'x-greenhouse' in header_name or 'greenhouse' in header_value:
                return 'greenhouse'
            if 'x-lever' in header_name or 'lever.co' in header_value:
                return 'lever'
            if 'workday' in header_name or 'workday' in header_value:
                return 'workday'
            if 'icims' in header_name or 'icims' in header_value:
                return 'icims'
            if 'taleo' in header_name or 'taleo' in header_value:
                return 'taleo'
            if 'smartrecruiters' in header_name or 'smartrecruiters' in header_value:
                return 'smartrecruiters'
            if 'ashby' in header_name or 'ashbyhq' in header_value:
                return 'ashby'
            if 'bamboohr' in header_name or 'bamboohr' in header_value:
                return 'bamboohr'
            if 'jazzhr' in header_name or 'jazzhr' in header_value or 'applytojob' in header_value:
                return 'jazzhr'
            if 'jobvite' in header_name or 'jobvite' in header_value:
                return 'jobvite'

        # Check body content for ATS signatures
        body_lower = body.lower()
        if 'powered by greenhouse' in body_lower or 'greenhouse.io' in body_lower:
            return 'greenhouse'
        if 'powered by lever' in body_lower or 'lever.co' in body_lower:
            return 'lever'
        if 'workday' in body_lower and ('myworkday' in body_lower or 'wd1.' in body_lower or 'wd3.' in body_lower or 'wd5.' in body_lower):
            return 'workday'
        if 'icims' in body_lower:
            return 'icims'
        if 'taleo' in body_lower or 'oracle recruiting' in body_lower:
            return 'taleo'
        if 'smartrecruiters' in body_lower:
            return 'smartrecruiters'
        if 'ashby' in body_lower:
            return 'ashby'
        if 'bamboohr' in body_lower:
            return 'bamboohr'
        if 'jazzhr' in body_lower or 'applytojob.com' in body_lower:
            return 'jazzhr'
        if 'jobvite' in body_lower:
            return 'jobvite'

        return None

    def extract_with_ats_patterns(self, ats: str, body: str, subject: Optional[str] = None) -> Dict[str, Optional[str]]:
        """Use ATS-specific patterns for better extraction

        Args:
            ats: ATS platform name
            body: Email body text
            subject: Email subject line (optional)

        Returns:
            Dict with extracted fields (company, position, application_id, job_url)
        """
        result = {
            'company': None,
            'position': None,
            'application_id': None,
            'job_url': None
        }

        if ats not in self.ats_patterns:
            return result

        patterns = self.ats_patterns[ats]

        # Extract company using ATS-specific patterns
        for pattern in patterns.get('company_patterns', []):
            match = re.search(pattern, body, re.IGNORECASE)
            if match:
                result['company'] = self.normalize_company_name(match.group(1).strip())
                break

        # Extract position using ATS-specific patterns
        for pattern in patterns.get('position_patterns', []):
            match = re.search(pattern, body, re.IGNORECASE)
            if match:
                result['position'] = match.group(1).strip()
                break

        # Extract application ID using ATS-specific patterns
        for pattern in patterns.get('app_id_patterns', []):
            match = re.search(pattern, body, re.IGNORECASE)
            if match:
                result['application_id'] = match.group(1).strip()
                break

        # Extract job URL - look for ATS-specific URL patterns
        url_pattern = r'https?://[^\s<>\'"]+(?:' + ats + r'|job|career|position|apply)[^\s<>\'"]*'
        urls = re.findall(url_pattern, body, re.IGNORECASE)
        if urls:
            result['job_url'] = urls[0]
        else:
            # Fallback to general URL extraction
            general_url_pattern = r'https?://[^\s<>\'"]+(?:job|career|position|application|apply)[^\s<>\'"]*'
            urls = re.findall(general_url_pattern, body, re.IGNORECASE)
            if urls:
                result['job_url'] = urls[0]

        return result

    def extract_company_from_email_domain(self, headers: list) -> Optional[str]:
        """Extract company name from sender email domain as fallback

        Args:
            headers: List of email header dicts

        Returns:
            Company name derived from domain, or None
        """
        for header in headers:
            if header['name'] == 'From':
                # Extract email address
                email_match = re.search(r'<([^>]+)>|([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+)', header['value'])
                if email_match:
                    email = email_match.group(1) or email_match.group(2)
                    # Extract domain
                    domain_match = re.search(r'@([a-zA-Z0-9.-]+)', email)
                    if domain_match:
                        domain = domain_match.group(1).lower()
                        # Skip generic email services and ATS platforms
                        generic_domains = [
                            'gmail.com', 'yahoo.com', 'hotmail.com', 'outlook.com',
                            'greenhouse.io', 'lever.co', 'workday.com', 'icims.com',
                            'taleo.net', 'jobvite.com', 'smartrecruiters.com',
                            'breezy.hr', 'ashbyhq.com', 'workable.com'
                        ]
                        if domain not in generic_domains:
                            # Extract company name from domain
                            # Remove common suffixes like .com, .io, .co, etc.
                            company = domain.split('.')[0]
                            # Capitalize first letter
                            return company.capitalize()
        return None

    def extract_position_from_subject(self, subject: str) -> Optional[str]:
        """Extract position title from subject line

        Args:
            subject: Email subject line

        Returns:
            Position title or None
        """
        # Try "Subject - Position Title" pattern
        if ' - ' in subject:
            parts = subject.split(' - ')
            if len(parts) >= 2:
                position = parts[-1].strip()
                # Basic validation
                if len(position) > 5 and not position.startswith('Re:'):
                    return position

        return None

    def parse_body_text(self, body: str) -> Dict[str, Optional[str]]:
        """Parse email body text for application data

        Args:
            body: Email body text

        Returns:
            Dict with extracted fields
        """
        result = {
            'company': None,
            'position': None,
            'application_id': None,
            'job_url': None
        }

        # Extract company
        for pattern in self.company_patterns:
            match = re.search(pattern, body, re.IGNORECASE)
            if match:
                result['company'] = self.normalize_company_name(match.group(1).strip())
                break

        # Extract position
        for pattern in self.position_patterns:
            match = re.search(pattern, body, re.IGNORECASE)
            if match:
                result['position'] = match.group(1).strip()
                break

        # Extract application ID
        for pattern in self.app_id_patterns:
            match = re.search(pattern, body)
            if match:
                result['application_id'] = match.group(1).strip()
                break

        # Extract job URL
        url_pattern = r'https?://[^\s<>]+'
        urls = re.findall(url_pattern, body)
        if urls:
            # Prefer URLs with job-related keywords
            for url in urls:
                if any(keyword in url.lower() for keyword in ['job', 'career', 'position', 'application']):
                    result['job_url'] = url
                    break
            if not result['job_url']:
                result['job_url'] = urls[0]

        return result

    def extract_email_body(self, message: Dict[str, Any]) -> str:
        """Extract plain text body from Gmail message

        Args:
            message: Gmail message dict

        Returns:
            Plain text body
        """
        if message is None:
            return ""

        payload = message.get('payload', {})

        # Try to get plain text body
        if 'parts' in payload:
            for part in payload['parts']:
                if part['mimeType'] == 'text/plain':
                    data = part['body'].get('data', '')
                    if data:
                        return base64.urlsafe_b64decode(data).decode('utf-8')

        # Fallback to single body
        data = payload.get('body', {}).get('data', '')
        if data:
            return base64.urlsafe_b64decode(data).decode('utf-8')

        return ''

    def parse_email(self, message: Dict[str, Any]) -> Dict[str, Optional[str]]:
        """Parse full email message

        Args:
            message: Gmail message dict

        Returns:
            Dict with all extracted fields
        """
        if message is None:
            return {
                'company': None,
                'position': None,
                'application_id': None,
                'job_url': None,
                'email_id': None,
                'subject': None,
                'ats_platform': None
            }

        headers = message.get('payload', {}).get('headers', [])

        # Get subject
        subject = None
        for header in headers:
            if header['name'] == 'Subject':
                subject = header['value']
                break

        # Extract body
        body = self.extract_email_body(message)

        # Detect ATS platform first
        ats_platform = self.detect_ats_platform(headers, body)

        # Try ATS-specific extraction first if we detected a platform
        if ats_platform:
            result = self.extract_with_ats_patterns(ats_platform, body, subject)
        else:
            result = {
                'company': None,
                'position': None,
                'application_id': None,
                'job_url': None
            }

        # Fall back to generic parsing for any missing fields
        if not result['company'] or not result['position'] or not result['application_id']:
            generic_result = self.parse_body_text(body)
            if not result['company']:
                result['company'] = generic_result['company']
            if not result['position']:
                result['position'] = generic_result['position']
            if not result['application_id']:
                result['application_id'] = generic_result['application_id']
            if not result['job_url']:
                result['job_url'] = generic_result['job_url']

        # Try to extract from headers/subject if body parsing incomplete
        if not result['company']:
            result['company'] = self.extract_company_from_headers(headers)

        # Fallback: try to extract company from email domain
        if not result['company']:
            result['company'] = self.extract_company_from_email_domain(headers)

        if not result['position'] and subject:
            result['position'] = self.extract_position_from_subject(subject)

        # Add email metadata
        result['email_id'] = message['id']
        result['subject'] = subject
        result['ats_platform'] = ats_platform

        return result
