"""Tests for duplicate detection"""

import pytest
from gmail_tracker.duplicate_detector import DuplicateDetector, strip_company_suffix
from gmail_tracker.config import Config


def test_detector_initializes():
    """Test that detector initializes"""
    config = Config()
    detector = DuplicateDetector(config)
    assert detector is not None


def test_fuzzy_match_similar_names():
    """Test fuzzy matching for similar company names"""
    config = Config()
    detector = DuplicateDetector(config)

    # Test similar names
    assert detector.fuzzy_match("Cloudflare", "CloudFlare") > 0.85
    assert detector.fuzzy_match("Huntress Labs", "Huntress") > 0.70


def test_fuzzy_match_different_names():
    """Test fuzzy matching for different company names"""
    config = Config()
    detector = DuplicateDetector(config)

    # Test different names
    assert detector.fuzzy_match("Cloudflare", "Microsoft") < 0.50


def test_check_alias_match():
    """Test checking company aliases"""
    config = Config()
    detector = DuplicateDetector(config)

    # Would need actual aliases loaded
    assert hasattr(detector, 'check_alias')


# Tests for suffix stripping functionality
class TestSuffixStripping:
    """Tests for company suffix stripping"""

    def test_strip_inc(self):
        """Test stripping Inc suffix"""
        assert strip_company_suffix("Cloudflare Inc") == "Cloudflare"
        assert strip_company_suffix("Cloudflare Inc.") == "Cloudflare"

    def test_strip_llc(self):
        """Test stripping LLC suffix"""
        assert strip_company_suffix("Google LLC") == "Google"
        assert strip_company_suffix("Google LLC.") == "Google"

    def test_strip_corp(self):
        """Test stripping Corp suffix"""
        assert strip_company_suffix("Microsoft Corp") == "Microsoft"
        assert strip_company_suffix("Microsoft Corp.") == "Microsoft"

    def test_strip_corporation(self):
        """Test stripping Corporation suffix"""
        assert strip_company_suffix("Microsoft Corporation") == "Microsoft"

    def test_strip_labs(self):
        """Test stripping Labs suffix"""
        assert strip_company_suffix("Huntress Labs") == "Huntress"

    def test_strip_co(self):
        """Test stripping Co suffix"""
        assert strip_company_suffix("Example Co") == "Example"
        assert strip_company_suffix("Example Co.") == "Example"

    def test_no_suffix(self):
        """Test names without suffixes are unchanged"""
        assert strip_company_suffix("Cloudflare") == "Cloudflare"
        assert strip_company_suffix("Meta") == "Meta"

    def test_case_insensitive(self):
        """Test suffix stripping is case insensitive"""
        assert strip_company_suffix("Cloudflare INC") == "Cloudflare"
        assert strip_company_suffix("Cloudflare inc") == "Cloudflare"
        assert strip_company_suffix("Google llc") == "Google"


class TestFuzzyMatchWithSuffixes:
    """Tests for fuzzy matching with suffix stripping"""

    def test_match_with_inc_suffix(self):
        """Test matching company with Inc suffix to base name"""
        config = Config()
        detector = DuplicateDetector(config)

        # Should match perfectly after suffix stripping
        assert detector.fuzzy_match("Cloudflare Inc", "Cloudflare") == 1.0
        assert detector.fuzzy_match("Cloudflare", "Cloudflare Inc.") == 1.0

    def test_match_with_llc_suffix(self):
        """Test matching company with LLC suffix to base name"""
        config = Config()
        detector = DuplicateDetector(config)

        assert detector.fuzzy_match("Google LLC", "Google") == 1.0

    def test_match_with_corporation_suffix(self):
        """Test matching company with Corporation suffix to base name"""
        config = Config()
        detector = DuplicateDetector(config)

        assert detector.fuzzy_match("Microsoft Corporation", "Microsoft") == 1.0

    def test_match_with_labs_suffix(self):
        """Test matching company with Labs suffix to base name"""
        config = Config()
        detector = DuplicateDetector(config)

        assert detector.fuzzy_match("Huntress Labs", "Huntress") == 1.0

    def test_match_different_suffixes(self):
        """Test matching same company with different suffixes"""
        config = Config()
        detector = DuplicateDetector(config)

        # Both should normalize to "Example"
        assert detector.fuzzy_match("Example Inc", "Example LLC") == 1.0
        assert detector.fuzzy_match("Example Corp", "Example Corporation") == 1.0


class TestCompanyAliases:
    """Tests for company alias detection"""

    def test_alias_google(self):
        """Test Google alias detection"""
        config = Config()
        detector = DuplicateDetector(config)

        assert detector.check_alias("Google") == "Alphabet"
        assert detector.check_alias("GOOG") == "Alphabet"
        assert detector.check_alias("Google LLC") == "Alphabet"

    def test_alias_facebook(self):
        """Test Facebook/Meta alias detection"""
        config = Config()
        detector = DuplicateDetector(config)

        assert detector.check_alias("Facebook") == "Meta"
        assert detector.check_alias("FB") == "Meta"
        assert detector.check_alias("Meta Platforms") == "Meta"

    def test_alias_amazon(self):
        """Test Amazon alias detection"""
        config = Config()
        detector = DuplicateDetector(config)

        assert detector.check_alias("AWS") == "Amazon"
        assert detector.check_alias("Amazon Web Services") == "Amazon"
        assert detector.check_alias("Amazon.com") == "Amazon"

    def test_alias_microsoft(self):
        """Test Microsoft alias detection"""
        config = Config()
        detector = DuplicateDetector(config)

        assert detector.check_alias("MSFT") == "Microsoft"
        assert detector.check_alias("Microsoft Corporation") == "Microsoft"

    def test_alias_case_insensitive(self):
        """Test alias matching is case insensitive"""
        config = Config()
        detector = DuplicateDetector(config)

        assert detector.check_alias("google") == "Alphabet"
        assert detector.check_alias("GOOGLE") == "Alphabet"
        assert detector.check_alias("GoOgLe") == "Alphabet"

    def test_alias_not_found(self):
        """Test non-alias returns None"""
        config = Config()
        detector = DuplicateDetector(config)

        assert detector.check_alias("Cloudflare") is None
        assert detector.check_alias("RandomCompany") is None


class TestFindBestMatch:
    """Tests for find_best_match with aliases and fuzzy matching"""

    def test_find_match_via_alias(self):
        """Test finding match through alias"""
        config = Config()
        detector = DuplicateDetector(config)

        existing = [
            {'name': 'Alphabet', 'id': 1},
            {'name': 'Meta', 'id': 2},
        ]

        # Should find Alphabet via Google alias
        match = detector.find_best_match("Google", existing)
        assert match is not None
        assert match['name'] == 'Alphabet'

    def test_find_match_via_fuzzy(self):
        """Test finding match through fuzzy matching with suffix stripping"""
        config = Config()
        detector = DuplicateDetector(config)

        existing = [
            {'name': 'Cloudflare', 'id': 1},
            {'name': 'Microsoft', 'id': 2},
        ]

        # Should match Cloudflare Inc to Cloudflare
        match = detector.find_best_match("Cloudflare Inc", existing)
        assert match is not None
        assert match['name'] == 'Cloudflare'

    def test_no_match_below_threshold(self):
        """Test that no match is returned below threshold"""
        config = Config()
        detector = DuplicateDetector(config)

        existing = [
            {'name': 'Cloudflare', 'id': 1},
        ]

        # Completely different company should not match
        match = detector.find_best_match("Netflix", existing)
        assert match is None
