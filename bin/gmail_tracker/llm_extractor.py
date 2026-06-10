"""LLM-based email extraction using Claude"""

import json
from typing import Dict, Optional
from anthropic import Anthropic

class LLMExtractor:
    """Extracts job application data using Claude LLM"""

    def __init__(self, config, api_key: str):
        """Initialize LLM extractor

        Args:
            config: Config object
            api_key: Anthropic API key
        """
        self.config = config
        self.client = Anthropic(api_key=api_key)
        self.model = config.llm['model']
        self.max_tokens = config.llm['max_tokens']
        self.temperature = config.llm['temperature']

    def build_prompt(self, email_text: str) -> str:
        """Build extraction prompt for Claude

        Args:
            email_text: Email body text

        Returns:
            Formatted prompt
        """
        prompt = f"""Extract job application data from this email confirmation.

Email content:
{email_text}

Extract the following fields:
- company_name: The company you applied to
- position_title: The job position/role title
- application_date: When you applied (if mentioned)
- application_id: Any reference/confirmation number
- job_url: Link to job posting (if present)

Return JSON with confidence scores (0.0-1.0) for each field.
If a field cannot be determined, set value to null and confidence to 0.0.

Example response:
{{
  "company_name": {{"value": "Cloudflare", "confidence": 0.95}},
  "position_title": {{"value": "Senior Sales Engineer", "confidence": 0.90}},
  "application_date": {{"value": "2026-01-15", "confidence": 0.80}},
  "application_id": {{"value": null, "confidence": 0.0}},
  "job_url": {{"value": "https://...", "confidence": 1.0}}
}}

Respond only with valid JSON, no other text."""

        return prompt

    def extract(self, email_text: str) -> Dict[str, Optional[str]]:
        """Extract data from email using Claude

        Args:
            email_text: Email body text

        Returns:
            Dict with extracted fields and confidence scores
        """
        prompt = self.build_prompt(email_text)

        try:
            # Call Claude API
            message = self.client.messages.create(
                model=self.model,
                max_tokens=self.max_tokens,
                temperature=self.temperature,
                system=[
                    {
                        "type": "text",
                        "text": "You are a job-application extraction assistant.",
                        "cache_control": {"type": "ephemeral"},
                    }
                ],
                messages=[
                    {"role": "user", "content": prompt}
                ]
            )

            # Parse response
            response_text = message.content[0].text
            data = json.loads(response_text)

            # Extract values and confidence
            result = {
                'company': data.get('company_name', {}).get('value'),
                'position': data.get('position_title', {}).get('value'),
                'application_date': data.get('application_date', {}).get('value'),
                'application_id': data.get('application_id', {}).get('value'),
                'job_url': data.get('job_url', {}).get('value'),
                'confidence': {
                    'company': data.get('company_name', {}).get('confidence', 0.0),
                    'position': data.get('position_title', {}).get('confidence', 0.0),
                }
            }

            return result

        except json.JSONDecodeError as e:
            raise RuntimeError(f"Failed to parse LLM response as JSON: {e}")
        except Exception as e:
            raise RuntimeError(f"LLM extraction failed: {e}")
