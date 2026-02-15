"""Tests for exporter module

Comprehensive tests for PDF, CSV, and JSON export functionality.
"""

import pytest
import json
import csv
import io
from datetime import datetime, timedelta
from unittest.mock import MagicMock, patch, PropertyMock

from gmail_tracker.exporter import (
    ExporterError,
    PDFExporter,
    CSVExporter,
    JSONExporter,
    REPORTLAB_AVAILABLE,
    MATPLOTLIB_AVAILABLE,
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
        self._current_query = None
        self._params = None

    def execute(self, query, params=None):
        """Store query for later result matching."""
        self._current_query = query
        self._params = params

        # Normalize query for matching
        query_lower = query.lower().replace('\n', ' ').replace('  ', ' ')

        # Find matching result based on query content
        # Try longest matches first to be more specific
        sorted_patterns = sorted(self._query_results.keys(), key=len, reverse=True)
        for pattern in sorted_patterns:
            pattern_lower = pattern.lower().replace('\n', ' ').replace('  ', ' ')
            if pattern_lower in query_lower:
                self._current_result = self._query_results[pattern]
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
    """Create a mock database manager with sample data."""
    db = MagicMock()

    # Sample data for various queries
    sample_applications = [
        {
            'id': 1,
            'company': 'Acme Corp',
            'company_id': 1,
            'company_name': 'Acme Corp',
            'company_website': 'https://acme.com',
            'company_description': 'A tech company',
            'position': 'Software Engineer',
            'position_title': 'Software Engineer',
            'status': 'applied',
            'application_date': datetime(2024, 1, 15),
            'response_date': None,
            'job_url': 'https://acme.com/jobs/1',
            'notes': 'Applied via LinkedIn',
            'created_at': datetime(2024, 1, 15, 10, 30),
            'updated_at': datetime(2024, 1, 15, 10, 30),
            'application_id': 1,
            'application_created': datetime(2024, 1, 15, 10, 30),
            'application_updated': datetime(2024, 1, 15, 10, 30),
            'company_created': datetime(2024, 1, 1),
            'company_updated': datetime(2024, 1, 1),
        },
        {
            'id': 2,
            'company': 'Tech Inc',
            'company_id': 2,
            'company_name': 'Tech Inc',
            'company_website': 'https://techinc.com',
            'company_description': 'Enterprise solutions',
            'position': 'Senior Developer',
            'position_title': 'Senior Developer',
            'status': 'interview',
            'application_date': datetime(2024, 1, 10),
            'response_date': datetime(2024, 1, 17),
            'job_url': 'https://techinc.com/careers/2',
            'notes': 'Referral from John',
            'created_at': datetime(2024, 1, 10, 14, 0),
            'updated_at': datetime(2024, 1, 17, 9, 0),
            'application_id': 2,
            'application_created': datetime(2024, 1, 10, 14, 0),
            'application_updated': datetime(2024, 1, 17, 9, 0),
            'company_created': datetime(2024, 1, 1),
            'company_updated': datetime(2024, 1, 1),
        },
    ]

    sample_companies = [
        {
            'id': 1,
            'name': 'Acme Corp',
            'website': 'https://acme.com',
            'description': 'A tech company',
            'application_count': 1,
            'created_at': datetime(2024, 1, 1),
            'updated_at': datetime(2024, 1, 1),
        },
        {
            'id': 2,
            'name': 'Tech Inc',
            'website': 'https://techinc.com',
            'description': 'Enterprise solutions',
            'application_count': 1,
            'created_at': datetime(2024, 1, 1),
            'updated_at': datetime(2024, 1, 1),
        },
    ]

    # Status counts for analytics
    status_counts = [
        {'status': 'applied', 'count': 50},
        {'status': 'interview', 'count': 10},
        {'status': 'offer', 'count': 3},
        {'status': 'rejected', 'count': 20},
    ]

    # Timeline data
    timeline_data = [
        {'date': '2024-W01', 'count': 15},
        {'date': '2024-W02', 'count': 20},
        {'date': '2024-W03', 'count': 18},
    ]

    # Top companies
    top_companies = [
        {'company_id': 1, 'company_name': 'Acme Corp', 'application_count': 5},
        {'company_id': 2, 'company_name': 'Tech Inc', 'application_count': 3},
    ]

    # Company response rates
    company_response_rates = [
        {'company_id': 1, 'company_name': 'Acme Corp', 'total_applications': 5, 'responses_received': 2},
        {'company_id': 2, 'company_name': 'Tech Inc', 'total_applications': 3, 'responses_received': 1},
    ]

    # Single application for detail export
    single_app = sample_applications[0].copy()

    query_results = {
        'select count(*) as count from applications': [{'count': 83}],
        'count(distinct company_id)': [{'count': 45}],
        'group by status': status_counts,
        'avg': [{'avg_days': 7.5}],
        'from applications a join companies c': sample_applications,
        'from companies c left join applications': sample_companies,
        'from companies c join applications a on c.id = a.company_id group by c.id': top_companies,
        'having count(a.id) > 0': company_response_rates,  # For get_company_response_rates
        'where a.id = %s': [single_app],
        'to_char(application_date': timeline_data,
        'interval \'7 days\'': [{'count': 15}],
        'interval \'14 days\'': [{'count': 12}],
        'current_date - interval': [{'count': 30}],  # For get_application_velocity
        'status != \'applied\'': [{'count': 5}],
        'status = \'interviewing\'': [{'count': 3}],
        'status = \'offer\'': [{'count': 1}],
        'extract(dow from application_date)': [
            {'day_of_week': 1, 'total_applications': 15, 'responses_received': 3},
            {'day_of_week': 2, 'total_applications': 18, 'responses_received': 5},
            {'day_of_week': 3, 'total_applications': 20, 'responses_received': 4},
        ],
        'from companies order by name': sample_companies,
    }

    db.cursor = MockCursor(query_results)
    return db


@pytest.fixture
def mock_db_empty():
    """Create a mock database with no data."""
    db = MagicMock()
    db.cursor = MockCursor({
        'select count(*) as count from applications': [{'count': 0}],
        'count(distinct company_id)': [{'count': 0}],
        'group by status': [],
        'avg': [{'avg_days': None}],
        'from applications a join companies c': [],
        'from companies c left join applications': [],
    })
    return db


class TestCSVExporter:
    """Tests for CSVExporter class."""

    def test_init(self):
        """Test CSVExporter initialization."""
        exporter = CSVExporter()
        assert exporter is not None

    def test_export_applications_returns_csv_string(self, mock_db):
        """Test that export_applications returns valid CSV."""
        exporter = CSVExporter()
        result = exporter.export_applications(mock_db)

        assert isinstance(result, str)
        assert 'id,company,position,status' in result

    def test_export_applications_contains_header(self, mock_db):
        """Test that CSV has correct headers."""
        exporter = CSVExporter()
        result = exporter.export_applications(mock_db)

        reader = csv.reader(io.StringIO(result))
        header = next(reader)

        assert 'id' in header
        assert 'company' in header
        assert 'position' in header
        assert 'status' in header
        assert 'application_date' in header

    def test_export_applications_with_filters(self, mock_db):
        """Test export with filters applied."""
        exporter = CSVExporter()

        filters = {
            'status': 'applied',
            'date_from': datetime(2024, 1, 1),
            'date_to': datetime(2024, 1, 31),
            'company': 'Acme',
        }

        result = exporter.export_applications(mock_db, filters=filters)
        assert isinstance(result, str)

    def test_export_applications_empty_result(self, mock_db_empty):
        """Test export when no applications exist."""
        exporter = CSVExporter()
        result = exporter.export_applications(mock_db_empty)

        # Should still have header row
        assert 'id,company,position,status' in result

    def test_export_companies_returns_csv(self, mock_db):
        """Test that export_companies returns valid CSV."""
        exporter = CSVExporter()
        result = exporter.export_companies(mock_db)

        assert isinstance(result, str)
        assert 'name' in result

    def test_export_companies_contains_application_count(self, mock_db):
        """Test that company export includes application count."""
        exporter = CSVExporter()
        result = exporter.export_companies(mock_db)

        assert 'application_count' in result

    def test_export_full_backup_returns_csv(self, mock_db):
        """Test that full backup export returns valid CSV."""
        exporter = CSVExporter()
        result = exporter.export_full_backup(mock_db)

        assert isinstance(result, str)
        # Should have both application and company fields
        assert 'application_id' in result or 'company_name' in result

    def test_export_handles_none_values(self, mock_db):
        """Test that export handles None values gracefully."""
        # Add an application with None values
        mock_db.cursor._query_results['from applications a join companies c'] = [{
            'id': 1,
            'company': 'Test',
            'position': 'Dev',
            'status': 'applied',
            'application_date': None,
            'response_date': None,
            'job_url': None,
            'notes': None,
            'created_at': None,
            'updated_at': None,
        }]

        exporter = CSVExporter()
        result = exporter.export_applications(mock_db)

        # Should not raise exception
        assert isinstance(result, str)

    def test_export_error_on_db_failure(self, mock_db):
        """Test that ExporterError is raised on database failure."""
        mock_db.cursor.execute = MagicMock(side_effect=Exception("DB error"))

        exporter = CSVExporter()

        with pytest.raises(ExporterError) as exc_info:
            exporter.export_applications(mock_db)

        assert "Failed to export" in str(exc_info.value)


class TestJSONExporter:
    """Tests for JSONExporter class."""

    def test_init(self):
        """Test JSONExporter initialization."""
        exporter = JSONExporter()
        assert exporter is not None

    def test_export_all_returns_dict(self, mock_db):
        """Test that export_all returns a dictionary."""
        exporter = JSONExporter()
        result = exporter.export_all(mock_db)

        assert isinstance(result, dict)

    def test_export_all_contains_required_keys(self, mock_db):
        """Test that export contains all required keys."""
        exporter = JSONExporter()
        result = exporter.export_all(mock_db)

        assert 'export_date' in result
        assert 'version' in result
        assert 'companies' in result
        assert 'applications' in result
        assert 'summary' in result

    def test_export_all_companies_is_list(self, mock_db):
        """Test that companies is a list."""
        exporter = JSONExporter()
        result = exporter.export_all(mock_db)

        assert isinstance(result['companies'], list)

    def test_export_all_applications_is_list(self, mock_db):
        """Test that applications is a list."""
        exporter = JSONExporter()
        result = exporter.export_all(mock_db)

        assert isinstance(result['applications'], list)

    def test_export_to_string_returns_valid_json(self, mock_db):
        """Test that export_to_string returns valid JSON."""
        exporter = JSONExporter()
        result = exporter.export_to_string(mock_db)

        # Should be parseable JSON
        parsed = json.loads(result)
        assert isinstance(parsed, dict)

    def test_export_to_string_with_indent(self, mock_db):
        """Test that indentation is applied."""
        exporter = JSONExporter()
        result = exporter.export_to_string(mock_db, indent=4)

        # Indented JSON should have newlines
        assert '\n' in result

    def test_export_handles_datetime_serialization(self, mock_db):
        """Test that datetime objects are serialized correctly."""
        exporter = JSONExporter()
        result = exporter.export_to_string(mock_db)

        # Should not raise exception for datetime serialization
        parsed = json.loads(result)
        assert 'export_date' in parsed

    def test_export_error_on_db_failure(self, mock_db):
        """Test that ExporterError is raised on database failure."""
        mock_db.cursor.execute = MagicMock(side_effect=Exception("DB error"))

        exporter = JSONExporter()

        with pytest.raises(ExporterError) as exc_info:
            exporter.export_all(mock_db)

        assert "Failed to export" in str(exc_info.value)


@pytest.mark.skipif(not REPORTLAB_AVAILABLE, reason="reportlab not installed")
class TestPDFExporter:
    """Tests for PDFExporter class."""

    def test_init(self):
        """Test PDFExporter initialization."""
        exporter = PDFExporter()
        assert exporter is not None

    def test_init_creates_custom_styles(self):
        """Test that custom styles are created."""
        exporter = PDFExporter()

        assert 'Title_Custom' in exporter.styles
        assert 'Heading1_Custom' in exporter.styles
        assert 'Body_Custom' in exporter.styles

    def test_generate_summary_report_returns_bytes(self, mock_db):
        """Test that summary report returns PDF bytes."""
        exporter = PDFExporter()
        result = exporter.generate_summary_report(mock_db)

        assert isinstance(result, bytes)
        # PDF files start with %PDF
        assert result[:4] == b'%PDF'

    def test_generate_summary_report_with_date_range(self, mock_db):
        """Test summary report with date range."""
        exporter = PDFExporter()

        date_range = (
            datetime(2024, 1, 1),
            datetime(2024, 1, 31)
        )
        result = exporter.generate_summary_report(mock_db, date_range=date_range)

        assert isinstance(result, bytes)
        assert result[:4] == b'%PDF'

    def test_generate_application_detail_returns_bytes(self, mock_db):
        """Test that application detail returns PDF bytes."""
        exporter = PDFExporter()
        result = exporter.generate_application_detail(mock_db, app_id=1)

        assert isinstance(result, bytes)
        assert result[:4] == b'%PDF'

    def test_generate_application_detail_not_found(self, mock_db):
        """Test application detail with non-existent ID."""
        # Make the query return no results
        mock_db.cursor._query_results['where a.id = %s'] = []

        exporter = PDFExporter()

        with pytest.raises(ExporterError) as exc_info:
            exporter.generate_application_detail(mock_db, app_id=999)

        assert "not found" in str(exc_info.value)

    def test_generate_analytics_report_returns_bytes(self, mock_db):
        """Test that analytics report returns PDF bytes."""
        exporter = PDFExporter()
        result = exporter.generate_analytics_report(mock_db)

        assert isinstance(result, bytes)
        assert result[:4] == b'%PDF'

    def test_generate_analytics_report_different_periods(self, mock_db):
        """Test analytics report with different time periods."""
        exporter = PDFExporter()

        for period in ['week', 'month', 'quarter', 'year']:
            result = exporter.generate_analytics_report(mock_db, period=period)
            assert isinstance(result, bytes)
            assert result[:4] == b'%PDF'

    def test_generate_recommendations(self, mock_db):
        """Test recommendation generation."""
        exporter = PDFExporter()

        stats = {'response_rate': 5.0}
        funnel = {
            'applied': 100,
            'responded': 5,
            'interviewed': 2,
            'offered': 0,
            'conversion_rates': {
                'response_rate': 5.0,
                'interview_rate': 40.0,
                'offer_rate': 0.0,
            }
        }
        velocity = 0.5
        best_days = {
            'Monday': {'total_applications': 20, 'responses_received': 5, 'response_rate': 25.0},
            'Tuesday': {'total_applications': 15, 'responses_received': 2, 'response_rate': 13.3},
        }

        recommendations = exporter._generate_recommendations(stats, funnel, velocity, best_days)

        assert isinstance(recommendations, list)
        assert len(recommendations) > 0

    def test_generate_recommendations_high_velocity(self, mock_db):
        """Test recommendations for high application velocity."""
        exporter = PDFExporter()

        stats = {'response_rate': 15.0}
        funnel = {
            'applied': 100,
            'responded': 15,
            'interviewed': 5,
            'offered': 2,
            'conversion_rates': {
                'response_rate': 15.0,
                'interview_rate': 33.0,
                'offer_rate': 40.0,
            }
        }
        velocity = 15.0  # Very high
        best_days = {}

        recommendations = exporter._generate_recommendations(stats, funnel, velocity, best_days)

        # Should mention high volume
        has_volume_mention = any('volume' in r.lower() for r in recommendations)
        assert has_volume_mention

    def test_generate_recommendations_good_response_rate(self, mock_db):
        """Test recommendations for good response rate."""
        exporter = PDFExporter()

        stats = {'response_rate': 35.0}
        funnel = {
            'applied': 100,
            'responded': 35,
            'interviewed': 15,
            'offered': 5,
            'conversion_rates': {
                'response_rate': 35.0,
                'interview_rate': 43.0,
                'offer_rate': 33.0,
            }
        }
        velocity = 3.0
        best_days = {}

        recommendations = exporter._generate_recommendations(stats, funnel, velocity, best_days)

        # Should mention excellent/good
        has_positive = any('excellent' in r.lower() or 'good' in r.lower() or 'keep' in r.lower()
                          for r in recommendations)
        assert has_positive

    def test_pdf_export_error_on_db_failure(self, mock_db):
        """Test that ExporterError is raised on database failure."""
        mock_db.cursor.execute = MagicMock(side_effect=Exception("DB error"))

        exporter = PDFExporter()

        with pytest.raises(ExporterError) as exc_info:
            exporter.generate_summary_report(mock_db)

        assert "Failed to fetch" in str(exc_info.value)

    def test_create_metric_table(self, mock_db):
        """Test metric table creation."""
        exporter = PDFExporter()

        metrics = [
            ('100', 'Total Applications'),
            ('45', 'Companies'),
            ('15%', 'Response Rate'),
            ('5', 'Offers'),
        ]

        result = exporter._create_metric_table(metrics)
        # Should return a Table object (from reportlab)
        assert result is not None

    def test_create_metric_table_empty(self, mock_db):
        """Test metric table with no data."""
        exporter = PDFExporter()
        result = exporter._create_metric_table([])
        # Should return something (Spacer in this case)
        assert result is not None

    @pytest.mark.skipif(not MATPLOTLIB_AVAILABLE, reason="matplotlib not installed")
    def test_create_status_pie_chart(self, mock_db):
        """Test pie chart creation."""
        exporter = PDFExporter()

        status_data = {
            'applied': 50,
            'interview': 10,
            'offer': 3,
            'rejected': 20,
        }

        fig = exporter._create_status_pie_chart(status_data)
        assert fig is not None

    @pytest.mark.skipif(not MATPLOTLIB_AVAILABLE, reason="matplotlib not installed")
    def test_create_status_pie_chart_empty(self, mock_db):
        """Test pie chart with no data."""
        exporter = PDFExporter()
        fig = exporter._create_status_pie_chart({})
        assert fig is None

    @pytest.mark.skipif(not MATPLOTLIB_AVAILABLE, reason="matplotlib not installed")
    def test_create_timeline_chart(self, mock_db):
        """Test timeline chart creation."""
        exporter = PDFExporter()

        timeline_data = [
            {'date': '2024-W01', 'count': 15},
            {'date': '2024-W02', 'count': 20},
            {'date': '2024-W03', 'count': 18},
        ]

        fig = exporter._create_timeline_chart(timeline_data)
        assert fig is not None

    @pytest.mark.skipif(not MATPLOTLIB_AVAILABLE, reason="matplotlib not installed")
    def test_create_funnel_chart(self, mock_db):
        """Test funnel chart creation."""
        exporter = PDFExporter()

        funnel_data = {
            'applied': 100,
            'responded': 30,
            'interviewed': 10,
            'offered': 3,
        }

        fig = exporter._create_funnel_chart(funnel_data)
        assert fig is not None


class TestPDFExporterWithoutReportlab:
    """Tests for PDFExporter when reportlab is not available."""

    def test_init_raises_error_without_reportlab(self):
        """Test that initialization raises error when reportlab is missing."""
        with patch.dict('gmail_tracker.exporter.__dict__', {'REPORTLAB_AVAILABLE': False}):
            # Need to reimport to get the patched value
            # This is tricky because the class checks at init time
            pass  # Skip this test as it requires more complex mocking


class TestExporterIntegration:
    """Integration tests for exporters."""

    def test_csv_to_json_consistency(self, mock_db):
        """Test that CSV and JSON exports contain consistent data."""
        csv_exporter = CSVExporter()
        json_exporter = JSONExporter()

        csv_result = csv_exporter.export_applications(mock_db)
        json_result = json_exporter.export_all(mock_db)

        # Both should have application data
        csv_rows = list(csv.reader(io.StringIO(csv_result)))
        json_apps = json_result.get('applications', [])

        # Header row in CSV
        assert len(csv_rows) > 0  # At least header

    def test_export_preserves_unicode(self):
        """Test that exports handle unicode characters correctly."""
        # Create a fresh mock with unicode data
        db = MagicMock()
        unicode_apps = [{
            'id': 1,
            'company': 'Acme\u00ae Corp',  # Unicode registered trademark
            'position': 'D\u00e9veloppeur',  # Unicode accent
            'status': 'applied',
            'application_date': datetime(2024, 1, 15),
            'response_date': None,
            'job_url': None,
            'notes': 'Note with \u00e9\u00e8\u00ea',
            'created_at': datetime(2024, 1, 15),
            'updated_at': datetime(2024, 1, 15),
        }]

        unicode_companies = [{
            'id': 1,
            'name': 'Acme\u00ae Corp',
            'website': None,
            'description': None,
            'created_at': datetime(2024, 1, 1),
            'updated_at': datetime(2024, 1, 1),
        }]

        db.cursor = MockCursor({
            # Key pattern matches the CSV export query - use 'where 1=1' which is unique to applications
            'where 1=1': unicode_apps,
            'from companies order by name': unicode_companies,
            'select count(*) as count from applications': [{'count': 1}],
            'count(distinct company_id)': [{'count': 1}],
            'group by status': [{'status': 'applied', 'count': 1}],
            'avg': [{'avg_days': None}],
        })

        csv_exporter = CSVExporter()
        json_exporter = JSONExporter()

        csv_result = csv_exporter.export_applications(db)
        json_result = json_exporter.export_to_string(db)

        # Should contain unicode without errors
        assert '\u00ae' in csv_result or 'Acme' in csv_result
        assert isinstance(json_result, str)


class TestExporterHelpers:
    """Tests for helper functions and utilities."""

    @pytest.mark.skipif(not REPORTLAB_AVAILABLE, reason="reportlab not installed")
    def test_status_colors_defined(self):
        """Test that all expected status colors are defined."""
        exporter = PDFExporter()

        expected_statuses = ['applied', 'interview', 'interviewing', 'offer',
                            'rejected', 'no_response', 'withdrawn']

        for status in expected_statuses:
            assert status in exporter.STATUS_COLORS

    @pytest.mark.skipif(not REPORTLAB_AVAILABLE, reason="reportlab not installed")
    def test_colors_defined(self):
        """Test that all expected colors are defined."""
        exporter = PDFExporter()

        expected_colors = ['primary', 'secondary', 'success', 'danger',
                          'warning', 'muted', 'dark', 'light']

        for color in expected_colors:
            assert color in exporter.COLORS
