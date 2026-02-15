"""Tests for LLM extraction"""

import pytest
from gmail_tracker.llm_extractor import LLMExtractor
from gmail_tracker.config import Config

def test_extractor_initializes():
    """Test that extractor initializes"""
    config = Config()
    extractor = LLMExtractor(config, api_key='test-key')
    assert extractor is not None

def test_build_extraction_prompt():
    """Test building extraction prompt"""
    config = Config()
    extractor = LLMExtractor(config, api_key='test-key')

    email_text = "Thank you for applying to Cloudflare..."
    prompt = extractor.build_prompt(email_text)

    assert "company_name" in prompt
    assert "position_title" in prompt
    assert email_text in prompt

@pytest.mark.skip(reason="Requires Anthropic API key")
def test_extract_from_email():
    """Test extracting data using LLM"""
    config = Config()
    extractor = LLMExtractor(config, api_key='real-api-key')

    email_text = """
    Thank you for applying to Cloudflare for Senior Sales Engineer.
    Application ID: CF-123
    """

    result = extractor.extract(email_text)
    assert result['company'] is not None
    assert result['position'] is not None
