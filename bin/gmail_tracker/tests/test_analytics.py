"""Tests for analytics module

Uses mock data to test analytics functions without requiring a database.
"""

import pytest
from unittest.mock import MagicMock, patch
from datetime import datetime, timedelta

from gmail_tracker.analytics import (
    AnalyticsError,
    get_summary_stats,
    get_applications_over_time,
    get_status_breakdown,
    get_top_companies,
    get_company_response_rates,
    get_best_application_days,
    get_application_velocity,
    get_funnel_stats,
    get_weekly_summary,
)


class MockCursor:
    """Mock cursor that returns predefined data based on query patterns."""

    def __init__(self, query_results=None):
        """Initialize with optional query result mappings.

        Args:
            query_results: Dict mapping query patterns to results
        """
        self._query_results = query_results or {}
        self._current_result = None
        self._result_index = 0

    def execute(self, query, params=None):
        """Store query for later result matching."""
        self._current_query = query
        self._params = params
        self._result_index = 0

        # Find matching result based on query content
        for pattern, result in self._query_results.items():
            if pattern.lower() in query.lower():
                self._current_result = result
                return

        self._current_result = []

    def fetchone(self):
        """Return first result."""
        if self._current_result and len(self._current_result) > 0:
            return self._current_result[0]
        return None

    def fetchall(self):
        """Return all results."""
        return self._current_result or []


@pytest.fixture
def mock_db():
    """Create a mock database manager."""
    db = MagicMock()
    db.cursor = MockCursor()
    return db


class TestGetSummaryStats:
    """Tests for get_summary_stats function."""

    def test_returns_all_required_fields(self, mock_db):
        """Verify all expected fields are returned."""
        mock_db.cursor = MockCursor({
            'count(*) as count from applications': [{'count': 100}],
            'count(distinct company_id)': [{'count': 45}],
            'group by status': [
                {'status': 'applied', 'count': 50},
                {'status': 'interviewing', 'count': 20},
                {'status': 'offer', 'count': 5},
                {'status': 'rejected', 'count': 25}
            ],
            'avg': [{'avg_days': 7.5}]
        })

        result = get_summary_stats(mock_db)

        assert 'total_applications' in result
        assert 'total_companies' in result
        assert 'interviews_scheduled' in result
        assert 'offers_received' in result
        assert 'rejection_count' in result
        assert 'pending_count' in result
        assert 'response_rate' in result
        assert 'average_response_time_days' in result

    def test_calculates_correct_values(self, mock_db):
        """Verify calculations are correct."""
        mock_db.cursor = MockCursor({
            'count(*) as count from applications': [{'count': 100}],
            'count(distinct company_id)': [{'count': 45}],
            'group by status': [
                {'status': 'applied', 'count': 50},
                {'status': 'interviewing', 'count': 20},
                {'status': 'offer', 'count': 5},
                {'status': 'rejected', 'count': 25}
            ],
            'avg': [{'avg_days': 7.5}]
        })

        result = get_summary_stats(mock_db)

        assert result['total_applications'] == 100
        assert result['total_companies'] == 45
        assert result['interviews_scheduled'] == 20
        assert result['offers_received'] == 5
        assert result['rejection_count'] == 25
        assert result['pending_count'] == 50
        assert result['response_rate'] == 50.0  # 50 responded out of 100
        assert result['average_response_time_days'] == 7.5

    def test_handles_empty_database(self, mock_db):
        """Handle case when no applications exist."""
        mock_db.cursor = MockCursor({
            'count(*) as count from applications': [{'count': 0}],
            'count(distinct company_id)': [{'count': 0}],
            'group by status': [],
            'avg': [{'avg_days': None}]
        })

        result = get_summary_stats(mock_db)

        assert result['total_applications'] == 0
        assert result['total_companies'] == 0
        assert result['response_rate'] == 0.0
        assert result['average_response_time_days'] is None

    def test_handles_database_error(self, mock_db):
        """Handle database errors gracefully."""
        mock_db.cursor.execute = MagicMock(
            side_effect=Exception("Database connection lost")
        )

        with pytest.raises(AnalyticsError) as exc_info:
            get_summary_stats(mock_db)

        assert "Failed to get summary stats" in str(exc_info.value)


class TestGetApplicationsOverTime:
    """Tests for get_applications_over_time function."""

    def test_returns_daily_data(self, mock_db):
        """Test daily grouping."""
        mock_db.cursor = MockCursor({
            "to_char(application_date, 'yyyy-mm-dd')": [
                {'date': '2026-01-15', 'count': 5},
                {'date': '2026-01-16', 'count': 3},
                {'date': '2026-01-17', 'count': 7}
            ]
        })

        result = get_applications_over_time(mock_db, period='day')

        assert isinstance(result, list)
        assert len(result) == 3
        assert result[0]['date'] == '2026-01-15'
        assert result[0]['count'] == 5

    def test_returns_weekly_data(self, mock_db):
        """Test weekly grouping."""
        mock_db.cursor = MockCursor({
            "to_char(application_date, 'iyyy": [
                {'date': '2026-W02', 'count': 12},
                {'date': '2026-W03', 'count': 8}
            ]
        })

        result = get_applications_over_time(mock_db, period='week')

        assert isinstance(result, list)
        assert len(result) == 2

    def test_returns_monthly_data(self, mock_db):
        """Test monthly grouping."""
        mock_db.cursor = MockCursor({
            "to_char(application_date, 'yyyy-mm')": [
                {'date': '2025-12', 'count': 25},
                {'date': '2026-01', 'count': 30}
            ]
        })

        result = get_applications_over_time(mock_db, period='month')

        assert isinstance(result, list)
        assert len(result) == 2

    def test_invalid_period_raises_error(self, mock_db):
        """Invalid period should raise AnalyticsError."""
        with pytest.raises(AnalyticsError) as exc_info:
            get_applications_over_time(mock_db, period='invalid')

        assert "Invalid period" in str(exc_info.value)

    def test_handles_empty_results(self, mock_db):
        """Handle case with no applications."""
        mock_db.cursor = MockCursor({})

        result = get_applications_over_time(mock_db, period='day')

        assert result == []


class TestGetStatusBreakdown:
    """Tests for get_status_breakdown function."""

    def test_returns_status_counts(self, mock_db):
        """Verify status counts are returned correctly."""
        mock_db.cursor = MockCursor({
            'group by status': [
                {'status': 'applied', 'count': 50},
                {'status': 'interviewing', 'count': 20},
                {'status': 'offer', 'count': 5},
                {'status': 'rejected', 'count': 25}
            ]
        })

        result = get_status_breakdown(mock_db)

        assert result['applied'] == 50
        assert result['interviewing'] == 20
        assert result['offer'] == 5
        assert result['rejected'] == 25

    def test_handles_empty_database(self, mock_db):
        """Handle case with no applications."""
        mock_db.cursor = MockCursor({'group by status': []})

        result = get_status_breakdown(mock_db)

        assert result == {}

    def test_handles_database_error(self, mock_db):
        """Handle database errors gracefully."""
        mock_db.cursor.execute = MagicMock(
            side_effect=Exception("Query failed")
        )

        with pytest.raises(AnalyticsError) as exc_info:
            get_status_breakdown(mock_db)

        assert "Failed to get status breakdown" in str(exc_info.value)


class TestGetTopCompanies:
    """Tests for get_top_companies function."""

    def test_returns_companies_sorted_by_count(self, mock_db):
        """Verify companies are returned sorted by application count."""
        mock_db.cursor = MockCursor({
            'order by application_count desc': [
                {'company_id': 1, 'company_name': 'Google', 'application_count': 10},
                {'company_id': 2, 'company_name': 'Meta', 'application_count': 8},
                {'company_id': 3, 'company_name': 'Amazon', 'application_count': 5}
            ]
        })

        result = get_top_companies(mock_db, limit=10)

        assert len(result) == 3
        assert result[0]['company_name'] == 'Google'
        assert result[0]['application_count'] == 10

    def test_respects_limit(self, mock_db):
        """Verify limit parameter is used."""
        mock_db.cursor = MockCursor({
            'order by application_count desc': [
                {'company_id': 1, 'company_name': 'Google', 'application_count': 10}
            ]
        })

        result = get_top_companies(mock_db, limit=1)

        # Verify limit was passed to query
        assert mock_db.cursor._params == (1,)

    def test_handles_empty_results(self, mock_db):
        """Handle case with no companies."""
        mock_db.cursor = MockCursor({})

        result = get_top_companies(mock_db)

        assert result == []


class TestGetCompanyResponseRates:
    """Tests for get_company_response_rates function."""

    def test_returns_response_rates(self, mock_db):
        """Verify response rates are calculated correctly."""
        mock_db.cursor = MockCursor({
            'having count(a.id) > 0': [
                {
                    'company_id': 1,
                    'company_name': 'Google',
                    'total_applications': 10,
                    'responses_received': 8
                },
                {
                    'company_id': 2,
                    'company_name': 'Meta',
                    'total_applications': 5,
                    'responses_received': 2
                }
            ]
        })

        result = get_company_response_rates(mock_db)

        assert len(result) == 2
        assert result[0]['company_name'] == 'Google'
        assert result[0]['response_rate'] == 80.0
        assert result[1]['response_rate'] == 40.0

    def test_handles_zero_applications(self, mock_db):
        """Handle edge case of zero applications."""
        mock_db.cursor = MockCursor({'having count(a.id) > 0': []})

        result = get_company_response_rates(mock_db)

        assert result == []


class TestGetBestApplicationDays:
    """Tests for get_best_application_days function."""

    def test_returns_day_stats(self, mock_db):
        """Verify day statistics are returned correctly."""
        mock_db.cursor = MockCursor({
            'extract(dow from application_date)': [
                {'day_of_week': 1, 'total_applications': 20, 'responses_received': 12},
                {'day_of_week': 2, 'total_applications': 25, 'responses_received': 10},
                {'day_of_week': 3, 'total_applications': 15, 'responses_received': 9}
            ]
        })

        result = get_best_application_days(mock_db)

        assert 'Monday' in result
        assert result['Monday']['total_applications'] == 20
        assert result['Monday']['responses_received'] == 12
        assert result['Monday']['response_rate'] == 60.0

    def test_calculates_response_rates(self, mock_db):
        """Verify response rate calculation."""
        mock_db.cursor = MockCursor({
            'extract(dow from application_date)': [
                {'day_of_week': 0, 'total_applications': 10, 'responses_received': 5}
            ]
        })

        result = get_best_application_days(mock_db)

        assert result['Sunday']['response_rate'] == 50.0

    def test_handles_no_data(self, mock_db):
        """Handle case with no applications."""
        mock_db.cursor = MockCursor({})

        result = get_best_application_days(mock_db)

        assert result == {}


class TestGetApplicationVelocity:
    """Tests for get_application_velocity function."""

    def test_calculates_velocity(self, mock_db):
        """Verify velocity calculation."""
        mock_db.cursor = MockCursor({
            'interval': [{'count': 60}]
        })

        result = get_application_velocity(mock_db, days=30)

        assert result == 2.0  # 60 applications / 30 days

    def test_uses_custom_period(self, mock_db):
        """Verify custom period is used."""
        mock_db.cursor = MockCursor({
            'interval': [{'count': 7}]
        })

        result = get_application_velocity(mock_db, days=7)

        assert result == 1.0  # 7 applications / 7 days

    def test_handles_zero_applications(self, mock_db):
        """Handle case with no applications in period."""
        mock_db.cursor = MockCursor({
            'interval': [{'count': 0}]
        })

        result = get_application_velocity(mock_db, days=30)

        assert result == 0.0

    def test_rounds_to_two_decimals(self, mock_db):
        """Verify result is rounded to 2 decimal places."""
        mock_db.cursor = MockCursor({
            'interval': [{'count': 10}]
        })

        result = get_application_velocity(mock_db, days=7)

        assert result == 1.43  # 10/7 = 1.428... -> 1.43


class TestGetFunnelStats:
    """Tests for get_funnel_stats function."""

    def test_returns_funnel_stages(self, mock_db):
        """Verify funnel stages are calculated correctly."""
        # First query: total count
        # Second query: status breakdown
        mock_db.cursor = MockCursor({
            'count(*) as count from applications': [{'count': 100}],
            'group by status': [
                {'status': 'applied', 'count': 50},
                {'status': 'interviewing', 'count': 25},
                {'status': 'offer', 'count': 5},
                {'status': 'rejected', 'count': 20}
            ]
        })

        result = get_funnel_stats(mock_db)

        assert result['applied'] == 100
        assert result['responded'] == 50  # 100 - 50 still in applied
        assert result['interviewed'] == 30  # 25 interviewing + 5 offers
        assert result['offered'] == 5

    def test_calculates_conversion_rates(self, mock_db):
        """Verify conversion rate calculations."""
        mock_db.cursor = MockCursor({
            'count(*) as count from applications': [{'count': 100}],
            'group by status': [
                {'status': 'applied', 'count': 50},
                {'status': 'interviewing', 'count': 25},
                {'status': 'offer', 'count': 5},
                {'status': 'rejected', 'count': 20}
            ]
        })

        result = get_funnel_stats(mock_db)

        assert 'conversion_rates' in result
        rates = result['conversion_rates']
        assert rates['response_rate'] == 50.0  # 50/100
        assert rates['interview_rate'] == 60.0  # 30/50
        assert rates['offer_rate'] == 16.7  # 5/30 rounded
        assert rates['overall_rate'] == 5.0  # 5/100

    def test_handles_empty_funnel(self, mock_db):
        """Handle case with no applications."""
        mock_db.cursor = MockCursor({
            'count(*) as count from applications': [{'count': 0}],
            'group by status': []
        })

        result = get_funnel_stats(mock_db)

        assert result['applied'] == 0
        assert result['conversion_rates']['response_rate'] == 0.0
        assert result['conversion_rates']['overall_rate'] == 0.0

    def test_handles_all_in_applied_stage(self, mock_db):
        """Handle case when all applications are still pending."""
        mock_db.cursor = MockCursor({
            'count(*) as count from applications': [{'count': 50}],
            'group by status': [
                {'status': 'applied', 'count': 50}
            ]
        })

        result = get_funnel_stats(mock_db)

        assert result['responded'] == 0
        assert result['interviewed'] == 0
        assert result['offered'] == 0


class TestGetWeeklySummary:
    """Tests for get_weekly_summary function."""

    def test_returns_weekly_stats(self, mock_db):
        """Verify weekly summary fields."""

        class WeeklySummaryCursor:
            """Custom cursor for weekly summary tests."""
            def __init__(self):
                self.call_count = 0

            def execute(self, query, params=None):
                self.call_count += 1
                self._call_num = self.call_count

            def fetchone(self):
                # Return values based on call order
                if self._call_num == 1:  # This week apps
                    return {'count': 15}
                elif self._call_num == 2:  # Last week apps
                    return {'count': 10}
                elif self._call_num == 3:  # Responses this week
                    return {'count': 5}
                elif self._call_num == 4:  # Interviews this week
                    return {'count': 3}
                elif self._call_num == 5:  # Offers this week
                    return {'count': 1}
                return {'count': 0}

        mock_db.cursor = WeeklySummaryCursor()
        result = get_weekly_summary(mock_db)

        assert 'applications_submitted' in result
        assert 'responses_received' in result
        assert 'interviews_scheduled' in result
        assert 'offers_received' in result
        assert 'comparison' in result

    def test_calculates_week_over_week_change(self, mock_db):
        """Verify week-over-week comparison."""

        class WoWChangeCursor:
            def __init__(self):
                self.call_count = 0

            def execute(self, query, params=None):
                self.call_count += 1
                self._call_num = self.call_count

            def fetchone(self):
                if self._call_num == 1:  # This week: 20
                    return {'count': 20}
                elif self._call_num == 2:  # Last week: 10
                    return {'count': 10}
                elif self._call_num == 3:  # Responses
                    return {'count': 5}
                elif self._call_num == 4:  # Interviews
                    return {'count': 3}
                elif self._call_num == 5:  # Offers
                    return {'count': 1}
                return {'count': 0}

        mock_db.cursor = WoWChangeCursor()
        result = get_weekly_summary(mock_db)

        # 20 this week vs 10 last week = 100% increase
        assert result['comparison']['week_over_week_change'] == 100.0

    def test_handles_no_previous_week_data(self, mock_db):
        """Handle case when last week had no applications."""

        class NoPreviousWeekCursor:
            def __init__(self):
                self.call_count = 0

            def execute(self, query, params=None):
                self.call_count += 1
                self._call_num = self.call_count

            def fetchone(self):
                if self._call_num == 1:  # This week: 10
                    return {'count': 10}
                elif self._call_num == 2:  # Last week: 0
                    return {'count': 0}
                elif self._call_num == 3:  # Responses
                    return {'count': 2}
                elif self._call_num == 4:  # Interviews
                    return {'count': 1}
                elif self._call_num == 5:  # Offers
                    return {'count': 0}
                return {'count': 0}

        mock_db.cursor = NoPreviousWeekCursor()
        result = get_weekly_summary(mock_db)

        assert result['comparison']['last_week_applications'] == 0
        assert result['comparison']['week_over_week_change'] == 100.0


class TestAnalyticsIntegration:
    """Integration tests using a more realistic mock setup."""

    @pytest.fixture
    def populated_db(self):
        """Create a mock database with realistic data."""
        db = MagicMock()

        # Create a more sophisticated mock that tracks query order
        class OrderedMockCursor:
            def __init__(self):
                self.call_count = 0
                self.query_history = []

            def execute(self, query, params=None):
                self.query_history.append(query)
                self.call_count += 1
                self._last_query = query.lower()
                self._params = params

            def fetchone(self):
                query = self._last_query
                if 'count(*) as count from applications' in query:
                    if 'distinct' not in query:
                        return {'count': 150}
                if 'count(distinct company_id)' in query:
                    return {'count': 75}
                if 'avg' in query:
                    return {'avg_days': 5.3}
                if 'interval' in query:
                    return {'count': 25}
                return {'count': 0}

            def fetchall(self):
                if 'group by status' in self._last_query:
                    return [
                        {'status': 'applied', 'count': 60},
                        {'status': 'interviewing', 'count': 40},
                        {'status': 'rejected', 'count': 35},
                        {'status': 'offer', 'count': 15}
                    ]
                return []

        db.cursor = OrderedMockCursor()
        return db

    def test_summary_stats_with_realistic_data(self, populated_db):
        """Test summary stats with realistic data patterns."""
        result = get_summary_stats(populated_db)

        assert result['total_applications'] == 150
        assert result['total_companies'] == 75
        assert result['interviews_scheduled'] == 40
        assert result['offers_received'] == 15
        assert result['rejection_count'] == 35
        assert result['pending_count'] == 60
        # Response rate: (150-60)/150 = 60%
        assert result['response_rate'] == 60.0
        assert result['average_response_time_days'] == 5.3

    def test_funnel_stats_with_realistic_data(self, populated_db):
        """Test funnel stats with realistic data patterns."""
        result = get_funnel_stats(populated_db)

        assert result['applied'] == 150
        assert result['responded'] == 90  # 150 - 60 still in applied
        assert result['interviewed'] == 55  # 40 interviewing + 15 offers
        assert result['offered'] == 15


class TestEdgeCases:
    """Tests for edge cases and error handling."""

    def test_handles_none_values_in_database(self, mock_db):
        """Handle NULL values from database."""
        mock_db.cursor = MockCursor({
            'count(*) as count from applications': [{'count': 10}],
            'count(distinct company_id)': [{'count': 5}],
            'group by status': [{'status': 'applied', 'count': 10}],
            'avg': [{'avg_days': None}]
        })

        result = get_summary_stats(mock_db)

        assert result['average_response_time_days'] is None

    def test_all_functions_have_error_handling(self, mock_db):
        """Verify all analytics functions handle errors properly."""
        mock_db.cursor.execute = MagicMock(
            side_effect=Exception("Database error")
        )

        functions = [
            (get_summary_stats, [mock_db]),
            (get_applications_over_time, [mock_db]),
            (get_status_breakdown, [mock_db]),
            (get_top_companies, [mock_db]),
            (get_company_response_rates, [mock_db]),
            (get_best_application_days, [mock_db]),
            (get_application_velocity, [mock_db]),
            (get_funnel_stats, [mock_db]),
            (get_weekly_summary, [mock_db]),
        ]

        for func, args in functions:
            with pytest.raises(AnalyticsError):
                func(*args)

    def test_large_numbers_handled(self, mock_db):
        """Handle large application counts."""
        mock_db.cursor = MockCursor({
            'count(*) as count from applications': [{'count': 10000}],
            'count(distinct company_id)': [{'count': 5000}],
            'group by status': [
                {'status': 'applied', 'count': 5000},
                {'status': 'interviewing', 'count': 3000},
                {'status': 'offer', 'count': 500},
                {'status': 'rejected', 'count': 1500}
            ],
            'avg': [{'avg_days': 12.5}]
        })

        result = get_summary_stats(mock_db)

        assert result['total_applications'] == 10000
        assert result['response_rate'] == 50.0
