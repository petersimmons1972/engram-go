"""Integration test for full workflow"""

import pytest
from datetime import datetime

@pytest.mark.skip(reason="Requires full setup - run manually")
def test_full_workflow():
    """Test complete workflow from Gmail to database"""
    from gmail_tracker.config import Config
    from gmail_tracker.app import GmailJobTracker

    config = Config()
    app = GmailJobTracker(config)

    # Run in dry-run mode
    result = app.run(dry_run=True)

    # Should complete successfully
    assert result == 0

    # Check stats
    assert app.stats['emails_processed'] >= 0

    print(f"\nIntegration test results:")
    print(f"  Emails processed: {app.stats['emails_processed']}")
    print(f"  Applications inserted: {app.stats['applications_inserted']}")
    print(f"  Duplicates found: {app.stats['duplicates_found']}")
    print(f"  Needs review: {app.stats['needs_review']}")
    print(f"  Errors: {app.stats['errors']}")
