"""Tests for email parsing functionality"""

import pytest
from gmail_tracker.email_parser import EmailParser
from gmail_tracker.config import Config

def test_parser_initializes():
    """Test that parser initializes with config"""
    config = Config()
    parser = EmailParser(config)
    assert parser is not None

def test_parse_company_from_sender():
    """Test extracting company name from sender"""
    config = Config()
    parser = EmailParser(config)

    # Test with sender domain
    headers = [
        {'name': 'From', 'value': 'Cloudflare Recruiting <jobs@greenhouse.io>'}
    ]

    company = parser.extract_company_from_headers(headers)
    assert company == 'Cloudflare'  # "Recruiting" suffix is removed

def test_parse_position_from_subject():
    """Test extracting position from subject line"""
    config = Config()
    parser = EmailParser(config)

    subject = "Application received - Senior Sales Engineer"
    position = parser.extract_position_from_subject(subject)

    assert position == "Senior Sales Engineer"

def test_parse_email_body():
    """Test extracting data from email body"""
    config = Config()
    parser = EmailParser(config)

    body = """
    Thank you for applying to Cloudflare for the position of Senior Sales Engineer.

    Application ID: CF-2026-12345
    Applied on: January 15, 2026
    """

    result = parser.parse_body_text(body)
    assert result['company'] == 'Cloudflare'
    assert result['position'] == 'Senior Sales Engineer'
    assert result['application_id'] == 'CF-2026-12345'


class TestLeverStylePatterns:
    """Tests for Lever-style and generic email patterns"""

    def test_interest_in_company_pattern(self):
        """Test 'interest in Company!' pattern"""
        config = Config()
        parser = EmailParser(config)

        body = "Thank you for your interest in Stripe!"
        result = parser.parse_body_text(body)
        assert result['company'] == 'Stripe'

    def test_at_company_pattern(self):
        """Test 'at Company.' pattern"""
        config = Config()
        parser = EmailParser(config)

        body = "We've received your application at Microsoft."
        result = parser.parse_body_text(body)
        assert result['company'] == 'Microsoft'

    def test_team_at_company_pattern(self):
        """Test 'team at Company' pattern"""
        config = Config()
        parser = EmailParser(config)

        body = "The hiring team at Acme Corp will review your application."
        result = parser.parse_body_text(body)
        assert result['company'] == 'Acme'

    def test_from_company_pattern(self):
        """Test 'from Company is/has/would' pattern"""
        config = Config()
        parser = EmailParser(config)

        body = "A recruiter from Google is excited to connect with you."
        result = parser.parse_body_text(body)
        assert result['company'] == 'Google'


class TestNewPositionPatterns:
    """Tests for additional position extraction patterns"""

    def test_application_for_position_pattern(self):
        """Test 'application for Position' pattern"""
        config = Config()
        parser = EmailParser(config)

        body = "We received your application for Software Engineer."
        result = parser.parse_body_text(body)
        assert result['position'] == 'Software Engineer'

    def test_application_for_position_with_comma(self):
        """Test 'application for Position,' pattern"""
        config = Config()
        parser = EmailParser(config)

        body = "We received your application for Senior Backend Developer, and will review it."
        result = parser.parse_body_text(body)
        assert result['position'] == 'Senior Backend Developer'

    def test_application_for_position_at(self):
        """Test 'application for Position at' pattern"""
        config = Config()
        parser = EmailParser(config)

        body = "We received your application for DevOps Engineer at Acme Corp."
        result = parser.parse_body_text(body)
        assert result['position'] == 'DevOps Engineer'

    def test_applying_for_position_pattern(self):
        """Test 'applying for Position' pattern"""
        config = Config()
        parser = EmailParser(config)

        body = "Thank you for applying for Data Scientist."
        result = parser.parse_body_text(body)
        assert result['position'] == 'Data Scientist'

    def test_applying_to_position_pattern(self):
        """Test 'applying to Position' pattern"""
        config = Config()
        parser = EmailParser(config)

        body = "Thank you for applying to Staff Engineer."
        result = parser.parse_body_text(body)
        assert result['position'] == 'Staff Engineer'

    def test_role_of_position_pattern(self):
        """Test 'role of Position' pattern"""
        config = Config()
        parser = EmailParser(config)

        body = "You have been considered for the role of SRE."
        result = parser.parse_body_text(body)
        assert result['position'] == 'SRE'

    def test_role_as_position_pattern(self):
        """Test 'role as Position' pattern"""
        config = Config()
        parser = EmailParser(config)

        body = "Your interest in the role as Platform Engineer."
        result = parser.parse_body_text(body)
        assert result['position'] == 'Platform Engineer'

    def test_interested_in_position_pattern(self):
        """Test 'interested in Position position/role' pattern"""
        config = Config()
        parser = EmailParser(config)

        body = "Thank you for being interested in the Backend Engineer position."
        result = parser.parse_body_text(body)
        assert result['position'] == 'Backend Engineer'

    def test_interested_in_role_pattern(self):
        """Test 'interested in Role role' pattern"""
        config = Config()
        parser = EmailParser(config)

        body = "Thank you for being interested in Frontend Developer role."
        result = parser.parse_body_text(body)
        assert result['position'] == 'Frontend Developer'


class TestCompanyFromDomain:
    """Tests for extracting company from email domain"""

    def test_extract_company_from_domain(self):
        """Test extracting company from non-generic domain"""
        config = Config()
        parser = EmailParser(config)

        headers = [
            {'name': 'From', 'value': 'careers@microsoft.com'}
        ]
        company = parser.extract_company_from_email_domain(headers)
        assert company == 'Microsoft'

    def test_skip_generic_email_domain(self):
        """Test that generic email domains are skipped"""
        config = Config()
        parser = EmailParser(config)

        headers = [
            {'name': 'From', 'value': 'recruiter@gmail.com'}
        ]
        company = parser.extract_company_from_email_domain(headers)
        assert company is None

    def test_skip_ats_domain(self):
        """Test that ATS platform domains are skipped"""
        config = Config()
        parser = EmailParser(config)

        headers = [
            {'name': 'From', 'value': 'noreply@greenhouse.io'}
        ]
        company = parser.extract_company_from_email_domain(headers)
        assert company is None

    def test_extract_from_angle_bracket_format(self):
        """Test extracting domain from 'Name <email>' format"""
        config = Config()
        parser = EmailParser(config)

        headers = [
            {'name': 'From', 'value': 'Stripe Recruiting <careers@stripe.com>'}
        ]
        company = parser.extract_company_from_email_domain(headers)
        assert company == 'Stripe'


class TestCompanyNormalization:
    """Tests for company name normalization"""

    def test_normalize_inc(self):
        """Test stripping 'Inc' suffix"""
        config = Config()
        parser = EmailParser(config)

        assert parser.normalize_company_name('Acme Inc') == 'Acme'
        assert parser.normalize_company_name('Acme Inc.') == 'Acme'
        assert parser.normalize_company_name('Acme, Inc') == 'Acme'
        assert parser.normalize_company_name('Acme, Inc.') == 'Acme'

    def test_normalize_llc(self):
        """Test stripping 'LLC' suffix"""
        config = Config()
        parser = EmailParser(config)

        assert parser.normalize_company_name('Acme LLC') == 'Acme'
        assert parser.normalize_company_name('Acme, LLC') == 'Acme'

    def test_normalize_corp(self):
        """Test stripping 'Corp' and 'Corporation' suffixes"""
        config = Config()
        parser = EmailParser(config)

        assert parser.normalize_company_name('Acme Corp') == 'Acme'
        assert parser.normalize_company_name('Acme Corp.') == 'Acme'
        assert parser.normalize_company_name('Acme Corporation') == 'Acme'

    def test_normalize_ltd(self):
        """Test stripping 'Ltd' and 'Limited' suffixes"""
        config = Config()
        parser = EmailParser(config)

        assert parser.normalize_company_name('Acme Ltd') == 'Acme'
        assert parser.normalize_company_name('Acme Ltd.') == 'Acme'
        assert parser.normalize_company_name('Acme Limited') == 'Acme'

    def test_normalize_preserves_name_without_suffix(self):
        """Test that names without suffixes are preserved"""
        config = Config()
        parser = EmailParser(config)

        assert parser.normalize_company_name('Google') == 'Google'
        assert parser.normalize_company_name('Microsoft') == 'Microsoft'

    def test_normalize_empty_and_none(self):
        """Test handling of empty and None values"""
        config = Config()
        parser = EmailParser(config)

        assert parser.normalize_company_name('') == ''
        assert parser.normalize_company_name(None) is None

    def test_company_extraction_with_normalization(self):
        """Test that company extraction normalizes names"""
        config = Config()
        parser = EmailParser(config)

        body = "Thank you for applying to Acme Corporation for the position."
        result = parser.parse_body_text(body)
        assert result['company'] == 'Acme'


class TestATSDetection:
    """Tests for ATS platform detection"""

    def test_detect_greenhouse_from_domain(self):
        """Test detecting Greenhouse from email domain"""
        config = Config()
        parser = EmailParser(config)

        headers = [{'name': 'From', 'value': 'Cloudflare <jobs@greenhouse.io>'}]
        ats = parser.detect_ats_platform(headers, "")
        assert ats == 'greenhouse'

    def test_detect_lever_from_domain(self):
        """Test detecting Lever from email domain"""
        config = Config()
        parser = EmailParser(config)

        headers = [{'name': 'From', 'value': 'Stripe Recruiting <jobs@lever.co>'}]
        ats = parser.detect_ats_platform(headers, "")
        assert ats == 'lever'

    def test_detect_workday_from_domain(self):
        """Test detecting Workday from email domain"""
        config = Config()
        parser = EmailParser(config)

        headers = [{'name': 'From', 'value': 'IBM Careers <noreply@myworkday.com>'}]
        ats = parser.detect_ats_platform(headers, "")
        assert ats == 'workday'

    def test_detect_icims_from_domain(self):
        """Test detecting iCIMS from email domain"""
        config = Config()
        parser = EmailParser(config)

        headers = [{'name': 'From', 'value': 'careers@icims.com'}]
        ats = parser.detect_ats_platform(headers, "")
        assert ats == 'icims'

    def test_detect_taleo_from_domain(self):
        """Test detecting Taleo from email domain"""
        config = Config()
        parser = EmailParser(config)

        headers = [{'name': 'From', 'value': 'Oracle Careers <recruiting@taleo.net>'}]
        ats = parser.detect_ats_platform(headers, "")
        assert ats == 'taleo'

    def test_detect_smartrecruiters_from_domain(self):
        """Test detecting SmartRecruiters from email domain"""
        config = Config()
        parser = EmailParser(config)

        headers = [{'name': 'From', 'value': 'Visa Careers <noreply@smartrecruiters.com>'}]
        ats = parser.detect_ats_platform(headers, "")
        assert ats == 'smartrecruiters'

    def test_detect_ashby_from_domain(self):
        """Test detecting Ashby from email domain"""
        config = Config()
        parser = EmailParser(config)

        headers = [{'name': 'From', 'value': 'Notion <jobs@ashbyhq.com>'}]
        ats = parser.detect_ats_platform(headers, "")
        assert ats == 'ashby'

    def test_detect_bamboohr_from_domain(self):
        """Test detecting BambooHR from email domain"""
        config = Config()
        parser = EmailParser(config)

        headers = [{'name': 'From', 'value': 'careers@bamboohr.com'}]
        ats = parser.detect_ats_platform(headers, "")
        assert ats == 'bamboohr'

    def test_detect_jazzhr_from_domain(self):
        """Test detecting JazzHR from email domain"""
        config = Config()
        parser = EmailParser(config)

        headers = [{'name': 'From', 'value': 'Startup Inc <jobs@jazzhr.com>'}]
        ats = parser.detect_ats_platform(headers, "")
        assert ats == 'jazzhr'

    def test_detect_jazzhr_from_applytojob(self):
        """Test detecting JazzHR from applytojob.com domain"""
        config = Config()
        parser = EmailParser(config)

        headers = [{'name': 'From', 'value': 'noreply@applytojob.com'}]
        ats = parser.detect_ats_platform(headers, "")
        assert ats == 'jazzhr'

    def test_detect_jobvite_from_domain(self):
        """Test detecting Jobvite from email domain"""
        config = Config()
        parser = EmailParser(config)

        headers = [{'name': 'From', 'value': 'Intel Careers <recruiting@jobvite.com>'}]
        ats = parser.detect_ats_platform(headers, "")
        assert ats == 'jobvite'

    def test_detect_greenhouse_from_body(self):
        """Test detecting Greenhouse from email body"""
        config = Config()
        parser = EmailParser(config)

        headers = [{'name': 'From', 'value': 'careers@company.com'}]
        body = "Thank you for applying. Powered by Greenhouse"
        ats = parser.detect_ats_platform(headers, body)
        assert ats == 'greenhouse'

    def test_detect_lever_from_body(self):
        """Test detecting Lever from email body"""
        config = Config()
        parser = EmailParser(config)

        headers = [{'name': 'From', 'value': 'careers@company.com'}]
        body = "Apply at https://jobs.lever.co/company"
        ats = parser.detect_ats_platform(headers, body)
        assert ats == 'lever'

    def test_detect_workday_from_body(self):
        """Test detecting Workday from email body with myworkday"""
        config = Config()
        parser = EmailParser(config)

        headers = [{'name': 'From', 'value': 'careers@company.com'}]
        body = "View your application at https://company.wd5.myworkday.com/recruiting"
        ats = parser.detect_ats_platform(headers, body)
        assert ats == 'workday'

    def test_no_ats_detected(self):
        """Test that unknown domains return None"""
        config = Config()
        parser = EmailParser(config)

        headers = [{'name': 'From', 'value': 'careers@company.com'}]
        body = "Thank you for your application."
        ats = parser.detect_ats_platform(headers, body)
        assert ats is None


class TestWorkdayPatterns:
    """Tests for Workday-specific extraction patterns"""

    def test_workday_company_extraction(self):
        """Test extracting company from Workday email"""
        config = Config()
        parser = EmailParser(config)

        body = """
        Thank you for your interest in Netflix!

        Your application has been received.
        Position: Senior Software Engineer
        Requisition ID: R-12345
        """
        result = parser.extract_with_ats_patterns('workday', body)
        assert result['company'] == 'Netflix'
        assert result['position'] == 'Senior Software Engineer'
        assert result['application_id'] == 'R-12345'

    def test_workday_job_title_pattern(self):
        """Test Job Title extraction"""
        config = Config()
        parser = EmailParser(config)

        body = """
        Thank you for applying to Amazon!
        Job Title: Cloud Solutions Architect
        """
        result = parser.extract_with_ats_patterns('workday', body)
        assert result['position'] == 'Cloud Solutions Architect'

    def test_workday_req_id_pattern(self):
        """Test Req ID extraction"""
        config = Config()
        parser = EmailParser(config)

        body = """
        Thank you for applying to Microsoft.
        Req ID: REQ-98765
        """
        result = parser.extract_with_ats_patterns('workday', body)
        assert result['application_id'] == 'REQ-98765'


class TestICIMSPatterns:
    """Tests for iCIMS-specific extraction patterns"""

    def test_icims_company_extraction(self):
        """Test extracting company from iCIMS email"""
        config = Config()
        parser = EmailParser(config)

        body = """
        Thank you for applying to Target!
        Position: Store Team Lead
        Job ID: 12345678
        """
        result = parser.extract_with_ats_patterns('icims', body)
        assert result['company'] == 'Target'
        assert result['position'] == 'Store Team Lead'
        assert result['application_id'] == '12345678'

    def test_icims_reference_number(self):
        """Test Reference Number extraction"""
        config = Config()
        parser = EmailParser(config)

        body = """
        Your application to Walmart has been received.
        Reference Number: WAL-2026-001234
        """
        result = parser.extract_with_ats_patterns('icims', body)
        assert result['application_id'] == 'WAL-2026-001234'


class TestTaleoPatterns:
    """Tests for Taleo (Oracle) specific extraction patterns"""

    def test_taleo_company_extraction(self):
        """Test extracting company from Taleo email"""
        config = Config()
        parser = EmailParser(config)

        body = """
        Thank you for your interest in Oracle!
        Job Title: Database Administrator
        Requisition Number: ORC-12345
        """
        result = parser.extract_with_ats_patterns('taleo', body)
        assert result['company'] == 'Oracle'
        assert result['position'] == 'Database Administrator'
        assert result['application_id'] == 'ORC-12345'

    def test_taleo_careers_pattern(self):
        """Test company extraction from Careers pattern"""
        config = Config()
        parser = EmailParser(config)

        body = """
        Boeing Careers
        Thank you for your application.
        Job Number: 789456
        """
        result = parser.extract_with_ats_patterns('taleo', body)
        assert result['company'] == 'Boeing'
        assert result['application_id'] == '789456'


class TestSmartRecruitersPatterns:
    """Tests for SmartRecruiters-specific extraction patterns"""

    def test_smartrecruiters_company_extraction(self):
        """Test extracting company from SmartRecruiters email"""
        config = Config()
        parser = EmailParser(config)

        body = """
        Thank you for applying to Visa!
        You applied for: Senior Product Manager
        Application ID: abc123-def456-789
        """
        result = parser.extract_with_ats_patterns('smartrecruiters', body)
        assert result['company'] == 'Visa'
        assert result['position'] == 'Senior Product Manager'
        assert result['application_id'] == 'abc123-def456-789'

    def test_smartrecruiters_via_pattern(self):
        """Test company extraction from 'via SmartRecruiters' pattern"""
        config = Config()
        parser = EmailParser(config)

        body = """
        Spotify via SmartRecruiters
        Your application has been received.
        """
        result = parser.extract_with_ats_patterns('smartrecruiters', body)
        assert result['company'] == 'Spotify'


class TestAshbyPatterns:
    """Tests for Ashby-specific extraction patterns"""

    def test_ashby_company_extraction(self):
        """Test extracting company from Ashby email"""
        config = Config()
        parser = EmailParser(config)

        body = """
        Thank you for applying to Notion!
        We received your application for the Senior Engineer role.
        Application ID: ashby-12345
        """
        result = parser.extract_with_ats_patterns('ashby', body)
        assert result['company'] == 'Notion'
        assert result['position'] == 'Senior Engineer'
        assert result['application_id'] == 'ashby-12345'

    def test_ashby_team_pattern(self):
        """Test team at Company pattern"""
        config = Config()
        parser = EmailParser(config)

        body = """
        The team at Linear will review your application.
        """
        result = parser.extract_with_ats_patterns('ashby', body)
        assert result['company'] == 'Linear'


class TestBambooHRPatterns:
    """Tests for BambooHR-specific extraction patterns"""

    def test_bamboohr_company_extraction(self):
        """Test extracting company from BambooHR email"""
        config = Config()
        parser = EmailParser(config)

        body = """
        Thank you for applying to Acme Startup!
        Position: Marketing Manager
        Application ID: 98765
        """
        result = parser.extract_with_ats_patterns('bamboohr', body)
        assert result['company'] == 'Acme Startup'
        assert result['position'] == 'Marketing Manager'
        assert result['application_id'] == '98765'

    def test_bamboohr_position_pattern(self):
        """Test 'for the X position' pattern"""
        config = Config()
        parser = EmailParser(config)

        body = """
        We have received your application for the Data Analyst position.
        """
        result = parser.extract_with_ats_patterns('bamboohr', body)
        assert result['position'] == 'Data Analyst'


class TestJazzHRPatterns:
    """Tests for JazzHR-specific extraction patterns"""

    def test_jazzhr_company_extraction(self):
        """Test extracting company from JazzHR email"""
        config = Config()
        parser = EmailParser(config)

        body = """
        Thank you for applying to Tech Startup Inc!
        Position: Full Stack Developer
        Job ID: js-dev-2026
        """
        result = parser.extract_with_ats_patterns('jazzhr', body)
        assert result['company'] == 'Tech Startup'
        assert result['position'] == 'Full Stack Developer'
        assert result['application_id'] == 'js-dev-2026'

    def test_jazzhr_opening_pattern(self):
        """Test 'for the X opening' pattern"""
        config = Config()
        parser = EmailParser(config)

        body = """
        We have received your application for the Sales Representative opening.
        """
        result = parser.extract_with_ats_patterns('jazzhr', body)
        assert result['position'] == 'Sales Representative'


class TestJobvitePatterns:
    """Tests for Jobvite-specific extraction patterns"""

    def test_jobvite_company_extraction(self):
        """Test extracting company from Jobvite email"""
        config = Config()
        parser = EmailParser(config)

        body = """
        Thank you for applying to Intel!
        Position: Hardware Engineer
        Requisition ID: REQ-2026-001
        """
        result = parser.extract_with_ats_patterns('jobvite', body)
        assert result['company'] == 'Intel'
        assert result['position'] == 'Hardware Engineer'
        assert result['application_id'] == 'REQ-2026-001'

    def test_jobvite_careers_pattern(self):
        """Test 'Company Careers' pattern"""
        config = Config()
        parser = EmailParser(config)

        body = """
        Cisco Careers
        Your application has been submitted.
        """
        result = parser.extract_with_ats_patterns('jobvite', body)
        assert result['company'] == 'Cisco'


class TestGreenhousePatterns:
    """Tests for Greenhouse-specific extraction patterns"""

    def test_greenhouse_company_extraction(self):
        """Test extracting company from Greenhouse email"""
        config = Config()
        parser = EmailParser(config)

        body = """
        Thank you for applying to Cloudflare!
        Applied for: Security Engineer
        Application ID: 12345678
        """
        result = parser.extract_with_ats_patterns('greenhouse', body)
        assert result['company'] == 'Cloudflare'
        assert result['position'] == 'Security Engineer'
        assert result['application_id'] == '12345678'


class TestLeverPatterns:
    """Tests for Lever-specific extraction patterns"""

    def test_lever_company_extraction(self):
        """Test extracting company from Lever email"""
        config = Config()
        parser = EmailParser(config)

        body = """
        Thank you for your interest in Stripe!
        You applied for the Senior Backend Engineer position.
        Application ID: abc-123-def
        """
        result = parser.extract_with_ats_patterns('lever', body)
        assert result['company'] == 'Stripe'
        assert result['position'] == 'Senior Backend Engineer'
        assert result['application_id'] == 'abc-123-def'


class TestATSIntegration:
    """Integration tests for full ATS parsing flow"""

    def test_parse_email_with_ats_detection(self):
        """Test that parse_email detects ATS and uses specific patterns"""
        import base64
        config = Config()
        parser = EmailParser(config)

        body_text = """
        Thank you for your interest in Netflix!

        Your application for Senior Software Engineer has been received.
        Requisition ID: R-12345

        Visit https://netflix.wd5.myworkday.com/jobs to check status.
        """

        message = {
            'id': 'msg123',
            'payload': {
                'headers': [
                    {'name': 'From', 'value': 'Netflix Careers <noreply@myworkday.com>'},
                    {'name': 'Subject', 'value': 'Application Received - Senior Software Engineer'},
                ],
                'body': {
                    'data': base64.urlsafe_b64encode(body_text.encode()).decode()
                }
            }
        }

        result = parser.parse_email(message)
        assert result['ats_platform'] == 'workday'
        assert result['company'] == 'Netflix'
        assert result['position'] == 'Senior Software Engineer'
        assert result['application_id'] == 'R-12345'
        assert 'workday' in result['job_url'].lower()

    def test_parse_email_fallback_to_generic(self):
        """Test that parse_email falls back to generic patterns when ATS-specific fails"""
        import base64
        config = Config()
        parser = EmailParser(config)

        body_text = """
        Thank you for applying to TechCorp for the position of Data Scientist.
        Application ID: TC-2026-001
        """

        message = {
            'id': 'msg456',
            'payload': {
                'headers': [
                    {'name': 'From', 'value': 'TechCorp HR <hr@techcorp.com>'},
                    {'name': 'Subject', 'value': 'Application Confirmation'},
                ],
                'body': {
                    'data': base64.urlsafe_b64encode(body_text.encode()).decode()
                }
            }
        }

        result = parser.parse_email(message)
        assert result['ats_platform'] is None
        assert result['company'] == 'TechCorp'
        assert result['position'] == 'Data Scientist'
        assert result['application_id'] == 'TC-2026-001'

    def test_parse_email_greenhouse_full_flow(self):
        """Test full parsing flow with Greenhouse email"""
        import base64
        config = Config()
        parser = EmailParser(config)

        body_text = """
        Thank you for applying to Cloudflare!

        We have received your application for Security Engineer.
        Application ID: 87654321

        View your application: https://boards.greenhouse.io/cloudflare/jobs/12345

        Powered by Greenhouse
        """

        message = {
            'id': 'msg789',
            'payload': {
                'headers': [
                    {'name': 'From', 'value': 'Cloudflare Recruiting <jobs@greenhouse.io>'},
                    {'name': 'Subject', 'value': 'Application received - Security Engineer'},
                ],
                'body': {
                    'data': base64.urlsafe_b64encode(body_text.encode()).decode()
                }
            }
        }

        result = parser.parse_email(message)
        assert result['ats_platform'] == 'greenhouse'
        assert result['company'] == 'Cloudflare'
        assert result['position'] == 'Security Engineer'
        assert result['application_id'] == '87654321'
        assert 'greenhouse' in result['job_url'].lower()


class TestKnownATSDomains:
    """Tests for the KNOWN_ATS_DOMAINS constant"""

    def test_all_ats_domains_present(self):
        """Test that all expected ATS domains are present"""
        config = Config()
        parser = EmailParser(config)

        expected_domains = [
            'greenhouse.io',
            'lever.co',
            'myworkday.com',
            'myworkdayjobs.com',
            'icims.com',
            'taleo.net',
            'smartrecruiters.com',
            'ashbyhq.com',
            'bamboohr.com',
            'jazzhr.com',
            'applytojob.com',
            'jobvite.com',
        ]

        for domain in expected_domains:
            assert domain in parser.KNOWN_ATS_DOMAINS, f"Missing domain: {domain}"

    def test_ats_patterns_exist_for_all_ats(self):
        """Test that patterns exist for all ATS platforms"""
        config = Config()
        parser = EmailParser(config)

        expected_ats = [
            'greenhouse', 'lever', 'workday', 'icims', 'taleo',
            'smartrecruiters', 'ashby', 'bamboohr', 'jazzhr', 'jobvite'
        ]

        for ats in expected_ats:
            assert ats in parser.ats_patterns, f"Missing patterns for ATS: {ats}"
            assert 'subject_patterns' in parser.ats_patterns[ats]
            assert 'company_patterns' in parser.ats_patterns[ats]
            assert 'position_patterns' in parser.ats_patterns[ats]
            assert 'app_id_patterns' in parser.ats_patterns[ats]
