"""Analytics module for job application tracking

Provides comprehensive statistics and insights about job search progress.
"""

from datetime import datetime, timedelta
from typing import Dict, Any, List, Optional
from collections import defaultdict

from .database import DatabaseManager


class AnalyticsError(Exception):
    """Raised when analytics operations fail"""
    pass


def get_summary_stats(db: DatabaseManager) -> Dict[str, Any]:
    """Return summary statistics for all job applications.

    Args:
        db: DatabaseManager instance with active connection

    Returns:
        Dictionary containing:
            - total_applications: Total number of applications
            - total_companies: Number of unique companies applied to
            - interviews_scheduled: Count of applications in interview stage
            - offers_received: Count of applications with offers
            - rejection_count: Count of rejected applications
            - pending_count: Count of applications still pending response
            - response_rate: Percentage of applications that received any response
            - average_response_time_days: Average days to first response (or None)

    Raises:
        AnalyticsError: If database query fails
    """
    try:
        # Total applications
        db.cursor.execute("SELECT COUNT(*) as count FROM applications")
        total_applications = db.cursor.fetchone()['count']

        # Total unique companies
        db.cursor.execute(
            "SELECT COUNT(DISTINCT company_id) as count FROM applications"
        )
        total_companies = db.cursor.fetchone()['count']

        # Status counts
        db.cursor.execute("""
            SELECT status, COUNT(*) as count
            FROM applications
            GROUP BY status
        """)
        status_counts = {row['status']: row['count'] for row in db.cursor.fetchall()}

        interviews_scheduled = status_counts.get('interviewing', 0)
        offers_received = status_counts.get('offer', 0)
        rejection_count = status_counts.get('rejected', 0)
        pending_count = status_counts.get('applied', 0)

        # Response rate: applications that moved beyond 'applied' status
        responded_count = total_applications - pending_count
        response_rate = (
            round((responded_count / total_applications) * 100, 1)
            if total_applications > 0 else 0.0
        )

        # Average response time (for applications with response_date)
        db.cursor.execute("""
            SELECT AVG(
                EXTRACT(EPOCH FROM (response_date - application_date)) / 86400
            ) as avg_days
            FROM applications
            WHERE response_date IS NOT NULL
        """)
        result = db.cursor.fetchone()
        avg_response_time = (
            round(result['avg_days'], 1)
            if result['avg_days'] is not None else None
        )

        return {
            'total_applications': total_applications,
            'total_companies': total_companies,
            'interviews_scheduled': interviews_scheduled,
            'offers_received': offers_received,
            'rejection_count': rejection_count,
            'pending_count': pending_count,
            'response_rate': response_rate,
            'average_response_time_days': avg_response_time
        }

    except Exception as e:
        raise AnalyticsError(f"Failed to get summary stats: {e}")


def get_applications_over_time(
    db: DatabaseManager,
    period: str = 'week'
) -> List[Dict[str, Any]]:
    """Get applications grouped by time period.

    Args:
        db: DatabaseManager instance with active connection
        period: Grouping period - 'day', 'week', or 'month'

    Returns:
        List of dictionaries with 'date' and 'count' keys, sorted by date.
        Date format depends on period:
            - day: 'YYYY-MM-DD'
            - week: 'YYYY-WXX' (ISO week)
            - month: 'YYYY-MM'

    Raises:
        AnalyticsError: If database query fails or invalid period specified
    """
    period_formats = {
        'day': "TO_CHAR(application_date, 'YYYY-MM-DD')",
        'week': "TO_CHAR(application_date, 'IYYY-\"W\"IW')",
        'month': "TO_CHAR(application_date, 'YYYY-MM')"
    }

    if period not in period_formats:
        raise AnalyticsError(
            f"Invalid period '{period}'. Must be one of: {list(period_formats.keys())}"
        )

    try:
        date_expr = period_formats[period]
        query = f"""
            SELECT {date_expr} as date, COUNT(*) as count
            FROM applications
            GROUP BY {date_expr}
            ORDER BY date
        """
        db.cursor.execute(query)
        return [dict(row) for row in db.cursor.fetchall()]

    except Exception as e:
        raise AnalyticsError(f"Failed to get applications over time: {e}")


def get_status_breakdown(db: DatabaseManager) -> Dict[str, int]:
    """Count applications by status.

    Args:
        db: DatabaseManager instance with active connection

    Returns:
        Dictionary mapping status names to counts.
        Example: {'applied': 50, 'interviewing': 10, 'offer': 2, 'rejected': 15}

    Raises:
        AnalyticsError: If database query fails
    """
    try:
        db.cursor.execute("""
            SELECT status, COUNT(*) as count
            FROM applications
            GROUP BY status
            ORDER BY count DESC
        """)
        return {row['status']: row['count'] for row in db.cursor.fetchall()}

    except Exception as e:
        raise AnalyticsError(f"Failed to get status breakdown: {e}")


def get_top_companies(
    db: DatabaseManager,
    limit: int = 10
) -> List[Dict[str, Any]]:
    """Get companies with most applications.

    Args:
        db: DatabaseManager instance with active connection
        limit: Maximum number of companies to return

    Returns:
        List of dictionaries with company info and application count:
            - company_id: Company ID
            - company_name: Company name
            - application_count: Number of applications

    Raises:
        AnalyticsError: If database query fails
    """
    try:
        db.cursor.execute("""
            SELECT
                c.id as company_id,
                c.name as company_name,
                COUNT(a.id) as application_count
            FROM companies c
            JOIN applications a ON c.id = a.company_id
            GROUP BY c.id, c.name
            ORDER BY application_count DESC
            LIMIT %s
        """, (limit,))
        return [dict(row) for row in db.cursor.fetchall()]

    except Exception as e:
        raise AnalyticsError(f"Failed to get top companies: {e}")


def get_company_response_rates(db: DatabaseManager) -> List[Dict[str, Any]]:
    """Get response rates by company.

    Args:
        db: DatabaseManager instance with active connection

    Returns:
        List of dictionaries with company response stats:
            - company_id: Company ID
            - company_name: Company name
            - total_applications: Total applications to this company
            - responses_received: Applications that got a response
            - response_rate: Percentage that got responses

    Raises:
        AnalyticsError: If database query fails
    """
    try:
        db.cursor.execute("""
            SELECT
                c.id as company_id,
                c.name as company_name,
                COUNT(a.id) as total_applications,
                COUNT(a.id) FILTER (WHERE a.status != 'applied') as responses_received
            FROM companies c
            JOIN applications a ON c.id = a.company_id
            GROUP BY c.id, c.name
            HAVING COUNT(a.id) > 0
            ORDER BY COUNT(a.id) FILTER (WHERE a.status != 'applied')::float /
                     COUNT(a.id) DESC,
                     COUNT(a.id) DESC
        """)

        results = []
        for row in db.cursor.fetchall():
            total = row['total_applications']
            responses = row['responses_received']
            rate = round((responses / total) * 100, 1) if total > 0 else 0.0

            results.append({
                'company_id': row['company_id'],
                'company_name': row['company_name'],
                'total_applications': total,
                'responses_received': responses,
                'response_rate': rate
            })

        return results

    except Exception as e:
        raise AnalyticsError(f"Failed to get company response rates: {e}")


def get_best_application_days(db: DatabaseManager) -> Dict[str, Dict[str, Any]]:
    """Analyze which days of the week have the best response rates.

    Args:
        db: DatabaseManager instance with active connection

    Returns:
        Dictionary mapping day names to stats:
            - total_applications: Applications submitted on this day
            - responses_received: How many got responses
            - response_rate: Percentage with responses

    Raises:
        AnalyticsError: If database query fails
    """
    day_names = [
        'Sunday', 'Monday', 'Tuesday', 'Wednesday',
        'Thursday', 'Friday', 'Saturday'
    ]

    try:
        db.cursor.execute("""
            SELECT
                EXTRACT(DOW FROM application_date)::int as day_of_week,
                COUNT(*) as total_applications,
                COUNT(*) FILTER (WHERE status != 'applied') as responses_received
            FROM applications
            GROUP BY EXTRACT(DOW FROM application_date)::int
            ORDER BY EXTRACT(DOW FROM application_date)::int
        """)

        results = {}
        for row in db.cursor.fetchall():
            day_index = row['day_of_week']
            day_name = day_names[day_index]
            total = row['total_applications']
            responses = row['responses_received']
            rate = round((responses / total) * 100, 1) if total > 0 else 0.0

            results[day_name] = {
                'total_applications': total,
                'responses_received': responses,
                'response_rate': rate
            }

        return results

    except Exception as e:
        raise AnalyticsError(f"Failed to get best application days: {e}")


def get_application_velocity(
    db: DatabaseManager,
    days: int = 30
) -> float:
    """Calculate average applications per day over a period.

    Args:
        db: DatabaseManager instance with active connection
        days: Number of days to look back

    Returns:
        Average applications per day (rounded to 2 decimal places)

    Raises:
        AnalyticsError: If database query fails
    """
    try:
        db.cursor.execute("""
            SELECT COUNT(*) as count
            FROM applications
            WHERE application_date >= CURRENT_DATE - INTERVAL '%s days'
        """, (days,))

        count = db.cursor.fetchone()['count']
        return round(count / days, 2)

    except Exception as e:
        raise AnalyticsError(f"Failed to get application velocity: {e}")


def get_funnel_stats(db: DatabaseManager) -> Dict[str, Any]:
    """Get conversion funnel statistics.

    Tracks progression through the application funnel:
    Applied -> Response -> Interview -> Offer

    Args:
        db: DatabaseManager instance with active connection

    Returns:
        Dictionary with funnel stages:
            - applied: Total applications submitted
            - responded: Applications that received any response
            - interviewed: Applications that reached interview stage
            - offered: Applications that received offers
            - conversion_rates: Stage-to-stage conversion percentages
                - response_rate: applied -> responded
                - interview_rate: responded -> interviewed
                - offer_rate: interviewed -> offered
                - overall_rate: applied -> offered

    Raises:
        AnalyticsError: If database query fails
    """
    try:
        # Get total applications
        db.cursor.execute("SELECT COUNT(*) as count FROM applications")
        total_applied = db.cursor.fetchone()['count']

        # Get status counts
        db.cursor.execute("""
            SELECT status, COUNT(*) as count
            FROM applications
            GROUP BY status
        """)
        status_counts = {row['status']: row['count'] for row in db.cursor.fetchall()}

        # Calculate funnel stages
        still_applied = status_counts.get('applied', 0)
        responded = total_applied - still_applied
        interviewed = status_counts.get('interviewing', 0) + status_counts.get('offer', 0)
        offered = status_counts.get('offer', 0)

        # Calculate conversion rates
        def safe_rate(numerator: int, denominator: int) -> float:
            """Calculate percentage safely, returning 0 if denominator is 0."""
            if denominator == 0:
                return 0.0
            return round((numerator / denominator) * 100, 1)

        return {
            'applied': total_applied,
            'responded': responded,
            'interviewed': interviewed,
            'offered': offered,
            'conversion_rates': {
                'response_rate': safe_rate(responded, total_applied),
                'interview_rate': safe_rate(interviewed, responded),
                'offer_rate': safe_rate(offered, interviewed),
                'overall_rate': safe_rate(offered, total_applied)
            }
        }

    except Exception as e:
        raise AnalyticsError(f"Failed to get funnel stats: {e}")


def get_weekly_summary(db: DatabaseManager) -> Dict[str, Any]:
    """Get a summary of activity over the past week.

    Args:
        db: DatabaseManager instance with active connection

    Returns:
        Dictionary with weekly stats:
            - applications_submitted: New applications this week
            - responses_received: Responses received this week
            - interviews_scheduled: New interviews this week
            - offers_received: New offers this week
            - comparison: Comparison to previous week

    Raises:
        AnalyticsError: If database query fails
    """
    try:
        # This week's applications
        db.cursor.execute("""
            SELECT COUNT(*) as count
            FROM applications
            WHERE application_date >= CURRENT_DATE - INTERVAL '7 days'
        """)
        this_week_apps = db.cursor.fetchone()['count']

        # Last week's applications (for comparison)
        db.cursor.execute("""
            SELECT COUNT(*) as count
            FROM applications
            WHERE application_date >= CURRENT_DATE - INTERVAL '14 days'
            AND application_date < CURRENT_DATE - INTERVAL '7 days'
        """)
        last_week_apps = db.cursor.fetchone()['count']

        # Responses this week (status changed and updated this week)
        db.cursor.execute("""
            SELECT COUNT(*) as count
            FROM applications
            WHERE status != 'applied'
            AND updated_at >= CURRENT_DATE - INTERVAL '7 days'
        """)
        responses_this_week = db.cursor.fetchone()['count']

        # Interviews scheduled this week
        db.cursor.execute("""
            SELECT COUNT(*) as count
            FROM applications
            WHERE status = 'interviewing'
            AND updated_at >= CURRENT_DATE - INTERVAL '7 days'
        """)
        interviews_this_week = db.cursor.fetchone()['count']

        # Offers this week
        db.cursor.execute("""
            SELECT COUNT(*) as count
            FROM applications
            WHERE status = 'offer'
            AND updated_at >= CURRENT_DATE - INTERVAL '7 days'
        """)
        offers_this_week = db.cursor.fetchone()['count']

        # Calculate week-over-week change
        if last_week_apps > 0:
            wow_change = round(
                ((this_week_apps - last_week_apps) / last_week_apps) * 100, 1
            )
        else:
            wow_change = 100.0 if this_week_apps > 0 else 0.0

        return {
            'applications_submitted': this_week_apps,
            'responses_received': responses_this_week,
            'interviews_scheduled': interviews_this_week,
            'offers_received': offers_this_week,
            'comparison': {
                'last_week_applications': last_week_apps,
                'week_over_week_change': wow_change
            }
        }

    except Exception as e:
        raise AnalyticsError(f"Failed to get weekly summary: {e}")
