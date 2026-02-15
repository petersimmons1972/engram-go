"""
Gmail Job Tracker Dashboard
A Streamlit-based dashboard for tracking job applications

Run with: streamlit run dashboard.py
"""

import sys
import os
from pathlib import Path

# Add parent directory to path so we can import gmail_tracker
sys.path.insert(0, str(Path(__file__).parent.parent))

import streamlit as st
import pandas as pd
import plotly.express as px
import plotly.graph_objects as go
from datetime import datetime, timedelta
from typing import Optional, Dict, Any, List, Tuple
import json

# Import existing modules
from gmail_tracker.config import Config, ConfigError
from gmail_tracker.database import DatabaseManager
from gmail_tracker.state import StateManager
from gmail_tracker.gmail_auth import GmailAuthenticator

# Page configuration must be first Streamlit command
st.set_page_config(
    page_title="Job Tracker",
    page_icon="briefcase",
    layout="wide",
    initial_sidebar_state="expanded"
)

# Custom CSS for dark theme and polish
CUSTOM_CSS = """
<style>
    /* Dark theme base */
    .stApp {
        background-color: #0e1117;
    }

    /* Metric cards */
    div[data-testid="metric-container"] {
        background-color: #1e2130;
        border: 1px solid #2d3250;
        border-radius: 10px;
        padding: 15px;
        box-shadow: 0 4px 6px rgba(0, 0, 0, 0.3);
    }

    div[data-testid="metric-container"] > label {
        color: #8b8fa3 !important;
        font-size: 0.9rem;
    }

    div[data-testid="metric-container"] > div {
        color: #ffffff !important;
    }

    /* Delta styling */
    div[data-testid="metric-container"] [data-testid="stMetricDelta"] {
        font-size: 0.85rem;
    }

    /* Cards and containers */
    .dashboard-card {
        background-color: #1e2130;
        border: 1px solid #2d3250;
        border-radius: 10px;
        padding: 20px;
        margin-bottom: 20px;
    }

    /* Tables */
    .dataframe {
        background-color: #1e2130 !important;
    }

    .dataframe th {
        background-color: #2d3250 !important;
        color: #ffffff !important;
    }

    .dataframe td {
        background-color: #1e2130 !important;
        color: #e0e0e0 !important;
    }

    /* Status badges */
    .status-applied {
        background-color: #3b82f6;
        color: white;
        padding: 4px 12px;
        border-radius: 12px;
        font-size: 0.8rem;
        font-weight: 500;
    }

    .status-interview {
        background-color: #8b5cf6;
        color: white;
        padding: 4px 12px;
        border-radius: 12px;
        font-size: 0.8rem;
        font-weight: 500;
    }

    .status-offer {
        background-color: #10b981;
        color: white;
        padding: 4px 12px;
        border-radius: 12px;
        font-size: 0.8rem;
        font-weight: 500;
    }

    .status-rejected {
        background-color: #ef4444;
        color: white;
        padding: 4px 12px;
        border-radius: 12px;
        font-size: 0.8rem;
        font-weight: 500;
    }

    .status-no_response {
        background-color: #6b7280;
        color: white;
        padding: 4px 12px;
        border-radius: 12px;
        font-size: 0.8rem;
        font-weight: 500;
    }

    /* Sidebar styling */
    section[data-testid="stSidebar"] {
        background-color: #161b22;
        border-right: 1px solid #2d3250;
    }

    section[data-testid="stSidebar"] .stButton > button {
        width: 100%;
        background-color: #2d3250;
        border: 1px solid #3d4470;
        color: #ffffff;
        transition: all 0.3s ease;
    }

    section[data-testid="stSidebar"] .stButton > button:hover {
        background-color: #3d4470;
        border-color: #4d5490;
    }

    /* Headers */
    h1, h2, h3 {
        color: #ffffff !important;
    }

    /* Activity feed items */
    .activity-item {
        padding: 12px;
        border-left: 3px solid #3b82f6;
        background-color: #1e2130;
        margin-bottom: 10px;
        border-radius: 0 8px 8px 0;
    }

    .activity-time {
        color: #6b7280;
        font-size: 0.8rem;
    }

    /* Charts */
    .js-plotly-plot {
        background-color: transparent !important;
    }

    /* Buttons */
    .stButton > button {
        background-color: #3b82f6;
        color: white;
        border: none;
        border-radius: 8px;
        padding: 8px 16px;
        font-weight: 500;
        transition: all 0.3s ease;
    }

    .stButton > button:hover {
        background-color: #2563eb;
        box-shadow: 0 4px 12px rgba(59, 130, 246, 0.4);
    }

    /* Input fields */
    .stTextInput > div > div > input,
    .stSelectbox > div > div > select,
    .stDateInput > div > div > input {
        background-color: #1e2130 !important;
        border: 1px solid #2d3250 !important;
        color: #ffffff !important;
        border-radius: 8px !important;
    }

    /* Expander */
    .streamlit-expanderHeader {
        background-color: #1e2130 !important;
        border: 1px solid #2d3250 !important;
        border-radius: 8px !important;
    }

    /* Tab styling */
    .stTabs [data-baseweb="tab-list"] {
        background-color: transparent;
        gap: 8px;
    }

    .stTabs [data-baseweb="tab"] {
        background-color: #1e2130;
        border-radius: 8px 8px 0 0;
        border: 1px solid #2d3250;
        border-bottom: none;
        color: #8b8fa3;
        padding: 10px 20px;
    }

    .stTabs [aria-selected="true"] {
        background-color: #2d3250 !important;
        color: #ffffff !important;
    }

    /* Success/Error messages */
    .stSuccess {
        background-color: rgba(16, 185, 129, 0.1) !important;
        border: 1px solid #10b981 !important;
    }

    .stError {
        background-color: rgba(239, 68, 68, 0.1) !important;
        border: 1px solid #ef4444 !important;
    }

    /* Info boxes */
    .stInfo {
        background-color: rgba(59, 130, 246, 0.1) !important;
        border: 1px solid #3b82f6 !important;
    }
</style>
"""

# Apply custom CSS
st.markdown(CUSTOM_CSS, unsafe_allow_html=True)


class DashboardAnalytics:
    """Analytics calculations for the dashboard"""

    def __init__(self, db: DatabaseManager):
        self.db = db

    def get_total_applications(self) -> int:
        """Get total number of applications"""
        query = "SELECT COUNT(*) as count FROM applications"
        self.db.cursor.execute(query)
        result = self.db.cursor.fetchone()
        return result['count'] if result else 0

    def get_applications_this_week(self) -> int:
        """Get applications submitted this week"""
        query = """
            SELECT COUNT(*) as count FROM applications
            WHERE application_date >= CURRENT_DATE - INTERVAL '7 days'
        """
        self.db.cursor.execute(query)
        result = self.db.cursor.fetchone()
        return result['count'] if result else 0

    def get_applications_last_week(self) -> int:
        """Get applications submitted last week"""
        query = """
            SELECT COUNT(*) as count FROM applications
            WHERE application_date >= CURRENT_DATE - INTERVAL '14 days'
            AND application_date < CURRENT_DATE - INTERVAL '7 days'
        """
        self.db.cursor.execute(query)
        result = self.db.cursor.fetchone()
        return result['count'] if result else 0

    def get_response_rate(self) -> Tuple[float, float]:
        """Get response rate and change from last month"""
        # Current response rate
        query = """
            SELECT
                COUNT(*) FILTER (WHERE status != 'applied' AND status != 'no_response') as responses,
                COUNT(*) as total
            FROM applications
        """
        self.db.cursor.execute(query)
        result = self.db.cursor.fetchone()
        current_rate = (result['responses'] / result['total'] * 100) if result['total'] > 0 else 0

        # Last month's rate for comparison
        query_prev = """
            SELECT
                COUNT(*) FILTER (WHERE status != 'applied' AND status != 'no_response') as responses,
                COUNT(*) as total
            FROM applications
            WHERE application_date < CURRENT_DATE - INTERVAL '30 days'
        """
        self.db.cursor.execute(query_prev)
        result_prev = self.db.cursor.fetchone()
        prev_rate = (result_prev['responses'] / result_prev['total'] * 100) if result_prev['total'] > 0 else 0

        delta = current_rate - prev_rate
        return round(current_rate, 1), round(delta, 1)

    def get_interview_count(self) -> int:
        """Get number of interviews"""
        query = "SELECT COUNT(*) as count FROM applications WHERE status = 'interview'"
        self.db.cursor.execute(query)
        result = self.db.cursor.fetchone()
        return result['count'] if result else 0

    def get_offer_count(self) -> int:
        """Get number of offers"""
        query = "SELECT COUNT(*) as count FROM applications WHERE status = 'offer'"
        self.db.cursor.execute(query)
        result = self.db.cursor.fetchone()
        return result['count'] if result else 0

    def get_status_breakdown(self) -> pd.DataFrame:
        """Get breakdown of applications by status"""
        query = """
            SELECT status, COUNT(*) as count
            FROM applications
            GROUP BY status
            ORDER BY count DESC
        """
        self.db.cursor.execute(query)
        results = self.db.cursor.fetchall()
        return pd.DataFrame(results) if results else pd.DataFrame(columns=['status', 'count'])

    def get_applications_over_time(self, days: int = 30) -> pd.DataFrame:
        """Get applications per day over time"""
        query = """
            SELECT
                application_date::date as date,
                COUNT(*) as count
            FROM applications
            WHERE application_date >= CURRENT_DATE - INTERVAL '%s days'
            GROUP BY application_date::date
            ORDER BY date
        """
        self.db.cursor.execute(query, (days,))
        results = self.db.cursor.fetchall()
        return pd.DataFrame(results) if results else pd.DataFrame(columns=['date', 'count'])

    def get_top_companies(self, limit: int = 10) -> pd.DataFrame:
        """Get companies with most applications"""
        query = """
            SELECT c.name as company, COUNT(*) as applications
            FROM applications a
            JOIN companies c ON a.company_id = c.id
            GROUP BY c.id, c.name
            ORDER BY applications DESC
            LIMIT %s
        """
        self.db.cursor.execute(query, (limit,))
        results = self.db.cursor.fetchall()
        return pd.DataFrame(results) if results else pd.DataFrame(columns=['company', 'applications'])

    def get_recent_applications(self, limit: int = 10) -> pd.DataFrame:
        """Get most recent applications"""
        query = """
            SELECT
                a.id,
                c.name as company,
                a.position_title as position,
                a.status,
                a.application_date,
                a.job_url
            FROM applications a
            JOIN companies c ON a.company_id = c.id
            ORDER BY a.application_date DESC, a.created_at DESC
            LIMIT %s
        """
        self.db.cursor.execute(query, (limit,))
        results = self.db.cursor.fetchall()
        return pd.DataFrame(results) if results else pd.DataFrame()

    def get_all_applications(self,
                            status_filter: str = None,
                            company_filter: str = None,
                            date_from: datetime = None,
                            date_to: datetime = None,
                            search_term: str = None) -> pd.DataFrame:
        """Get all applications with optional filters"""
        query = """
            SELECT
                a.id,
                c.name as company,
                a.position_title as position,
                a.status,
                a.application_date,
                a.job_url,
                a.notes,
                a.created_at,
                a.updated_at
            FROM applications a
            JOIN companies c ON a.company_id = c.id
            WHERE 1=1
        """
        params = []

        if status_filter and status_filter != "All":
            query += " AND a.status = %s"
            params.append(status_filter.lower())

        if company_filter and company_filter != "All":
            query += " AND c.name = %s"
            params.append(company_filter)

        if date_from:
            query += " AND a.application_date >= %s"
            params.append(date_from)

        if date_to:
            query += " AND a.application_date <= %s"
            params.append(date_to)

        if search_term:
            query += " AND (LOWER(c.name) LIKE %s OR LOWER(a.position_title) LIKE %s)"
            search_pattern = f"%{search_term.lower()}%"
            params.extend([search_pattern, search_pattern])

        query += " ORDER BY a.application_date DESC, a.created_at DESC"

        self.db.cursor.execute(query, params)
        results = self.db.cursor.fetchall()
        return pd.DataFrame(results) if results else pd.DataFrame()

    def get_all_companies(self) -> List[str]:
        """Get list of all company names"""
        query = "SELECT DISTINCT name FROM companies ORDER BY name"
        self.db.cursor.execute(query)
        results = self.db.cursor.fetchall()
        return [r['name'] for r in results] if results else []

    def get_all_statuses(self) -> List[str]:
        """Get list of all unique statuses"""
        query = "SELECT DISTINCT status FROM applications ORDER BY status"
        self.db.cursor.execute(query)
        results = self.db.cursor.fetchall()
        return [r['status'] for r in results] if results else []

    def get_response_rate_trend(self, months: int = 6) -> pd.DataFrame:
        """Get response rate trend over time"""
        query = """
            WITH monthly_stats AS (
                SELECT
                    DATE_TRUNC('month', application_date) as month,
                    COUNT(*) as total,
                    COUNT(*) FILTER (WHERE status != 'applied' AND status != 'no_response') as responses
                FROM applications
                WHERE application_date >= CURRENT_DATE - INTERVAL '%s months'
                GROUP BY DATE_TRUNC('month', application_date)
            )
            SELECT
                month,
                CASE WHEN total > 0 THEN ROUND((responses::numeric / total) * 100, 1) ELSE 0 END as response_rate
            FROM monthly_stats
            ORDER BY month
        """
        self.db.cursor.execute(query, (months,))
        results = self.db.cursor.fetchall()
        return pd.DataFrame(results) if results else pd.DataFrame(columns=['month', 'response_rate'])


class Dashboard:
    """Main dashboard application"""

    def __init__(self):
        self.config = None
        self.db = None
        self.analytics = None
        self.state = None
        self.gmail_auth = None

    def initialize(self) -> bool:
        """Initialize dashboard components"""
        try:
            self.config = Config()
            self.db = DatabaseManager(self.config.database, self.config.db_password)
            self.db.connect()
            self.analytics = DashboardAnalytics(self.db)
            self.state = StateManager()
            self.gmail_auth = GmailAuthenticator()
            return True
        except ConfigError as e:
            st.error(f"Configuration Error: {e}")
            return False
        except Exception as e:
            st.error(f"Failed to initialize: {e}")
            return False

    def cleanup(self):
        """Cleanup resources"""
        if self.db:
            self.db.disconnect()


@st.cache_data(ttl=60)
def get_cached_stats(_analytics: DashboardAnalytics) -> Dict[str, Any]:
    """Cache dashboard statistics for 60 seconds"""
    return {
        'total_applications': _analytics.get_total_applications(),
        'this_week': _analytics.get_applications_this_week(),
        'last_week': _analytics.get_applications_last_week(),
        'response_rate': _analytics.get_response_rate(),
        'interviews': _analytics.get_interview_count(),
        'offers': _analytics.get_offer_count(),
    }


@st.cache_data(ttl=60)
def get_cached_recent_applications(_analytics: DashboardAnalytics, limit: int = 10) -> pd.DataFrame:
    """Cache recent applications"""
    return _analytics.get_recent_applications(limit)


@st.cache_data(ttl=60)
def get_cached_status_breakdown(_analytics: DashboardAnalytics) -> pd.DataFrame:
    """Cache status breakdown"""
    return _analytics.get_status_breakdown()


@st.cache_data(ttl=60)
def get_cached_applications_over_time(_analytics: DashboardAnalytics, days: int) -> pd.DataFrame:
    """Cache applications over time"""
    return _analytics.get_applications_over_time(days)


@st.cache_data(ttl=60)
def get_cached_top_companies(_analytics: DashboardAnalytics, limit: int) -> pd.DataFrame:
    """Cache top companies"""
    return _analytics.get_top_companies(limit)


@st.cache_data(ttl=60)
def get_cached_response_rate_trend(_analytics: DashboardAnalytics, months: int) -> pd.DataFrame:
    """Cache response rate trend"""
    return _analytics.get_response_rate_trend(months)


def format_status_badge(status: str) -> str:
    """Format status as HTML badge"""
    status_lower = status.lower().replace(' ', '_')
    display_name = status.replace('_', ' ').title()
    return f'<span class="status-{status_lower}">{display_name}</span>'


def render_dashboard_page(analytics: DashboardAnalytics):
    """Render the main dashboard page"""
    st.title("Job Application Dashboard")

    # Get cached stats
    try:
        stats = get_cached_stats(analytics)
    except Exception as e:
        st.error(f"Failed to load statistics: {e}")
        return

    # Stats row with metrics
    col1, col2, col3, col4 = st.columns(4)

    with col1:
        week_delta = stats['this_week'] - stats['last_week']
        delta_str = f"+{week_delta}" if week_delta >= 0 else str(week_delta)
        st.metric(
            label="Total Applications",
            value=stats['total_applications'],
            delta=f"{delta_str} this week"
        )

    with col2:
        response_rate, rate_delta = stats['response_rate']
        delta_str = f"+{rate_delta}%" if rate_delta >= 0 else f"{rate_delta}%"
        st.metric(
            label="Response Rate",
            value=f"{response_rate}%",
            delta=delta_str
        )

    with col3:
        st.metric(
            label="Interviews",
            value=stats['interviews']
        )

    with col4:
        st.metric(
            label="Offers",
            value=stats['offers']
        )

    st.divider()

    # Two column layout for recent applications and activity
    col_left, col_right = st.columns([2, 1])

    with col_left:
        st.subheader("Recent Applications")

        recent_apps = get_cached_recent_applications(analytics, 10)

        if not recent_apps.empty:
            # Create a styled dataframe
            display_df = recent_apps[['company', 'position', 'status', 'application_date']].copy()
            display_df['application_date'] = pd.to_datetime(display_df['application_date']).dt.strftime('%Y-%m-%d')
            display_df.columns = ['Company', 'Position', 'Status', 'Applied']

            # Display using st.dataframe for better styling
            st.dataframe(
                display_df,
                use_container_width=True,
                hide_index=True,
                column_config={
                    "Company": st.column_config.TextColumn("Company", width="medium"),
                    "Position": st.column_config.TextColumn("Position", width="large"),
                    "Status": st.column_config.TextColumn("Status", width="small"),
                    "Applied": st.column_config.TextColumn("Applied", width="small"),
                }
            )
        else:
            st.info("No applications found. Start tracking your job applications!")

    with col_right:
        st.subheader("Quick Stats")

        # Status breakdown as small chart
        status_df = get_cached_status_breakdown(analytics)

        if not status_df.empty:
            # Color mapping for statuses
            color_map = {
                'applied': '#3b82f6',
                'interview': '#8b5cf6',
                'offer': '#10b981',
                'rejected': '#ef4444',
                'no_response': '#6b7280',
                'withdrawn': '#f59e0b'
            }

            fig = px.pie(
                status_df,
                values='count',
                names='status',
                hole=0.4,
                color='status',
                color_discrete_map=color_map
            )
            fig.update_layout(
                paper_bgcolor='rgba(0,0,0,0)',
                plot_bgcolor='rgba(0,0,0,0)',
                font_color='#ffffff',
                showlegend=True,
                legend=dict(
                    orientation="h",
                    yanchor="bottom",
                    y=-0.3,
                    xanchor="center",
                    x=0.5
                ),
                margin=dict(t=20, b=20, l=20, r=20)
            )
            fig.update_traces(textposition='inside', textinfo='value')
            st.plotly_chart(fig, use_container_width=True)
        else:
            st.info("No data available")

        # This week's activity
        st.subheader("This Week")
        apps_this_week = get_cached_applications_over_time(analytics, 7)

        if not apps_this_week.empty:
            total_this_week = apps_this_week['count'].sum()
            st.metric("Applications", total_this_week)
        else:
            st.metric("Applications", 0)


def render_applications_page(analytics: DashboardAnalytics, db: DatabaseManager):
    """Render the applications page"""
    st.title("Applications")

    # Filters in sidebar
    with st.sidebar:
        st.subheader("Filters")

        # Status filter
        statuses = ["All"] + analytics.get_all_statuses()
        status_filter = st.selectbox("Status", statuses)

        # Company filter
        companies = ["All"] + analytics.get_all_companies()
        company_filter = st.selectbox("Company", companies)

        # Date range
        st.write("Date Range")
        col1, col2 = st.columns(2)
        with col1:
            date_from = st.date_input("From", value=None)
        with col2:
            date_to = st.date_input("To", value=None)

        # Search
        search_term = st.text_input("Search", placeholder="Company or position...")

        # Clear filters button
        if st.button("Clear Filters"):
            st.rerun()

    # Get filtered applications
    apps_df = analytics.get_all_applications(
        status_filter=status_filter,
        company_filter=company_filter,
        date_from=date_from,
        date_to=date_to,
        search_term=search_term if search_term else None
    )

    # Display count
    st.write(f"Showing {len(apps_df)} applications")

    if not apps_df.empty:
        # Format for display
        display_df = apps_df[['id', 'company', 'position', 'status', 'application_date', 'job_url']].copy()
        display_df['application_date'] = pd.to_datetime(display_df['application_date']).dt.strftime('%Y-%m-%d')

        # Use expanders for details
        for idx, row in display_df.iterrows():
            with st.expander(f"{row['company']} - {row['position']} ({row['status']})"):
                col1, col2 = st.columns(2)

                with col1:
                    st.write(f"**Company:** {row['company']}")
                    st.write(f"**Position:** {row['position']}")
                    st.write(f"**Status:** {row['status'].replace('_', ' ').title()}")

                with col2:
                    st.write(f"**Applied:** {row['application_date']}")
                    if row['job_url']:
                        st.write(f"**Job URL:** [Link]({row['job_url']})")

                # Get notes from original dataframe
                notes = apps_df.loc[idx, 'notes'] if 'notes' in apps_df.columns else None
                if notes:
                    st.write(f"**Notes:** {notes}")

                # Status update form
                st.write("---")
                new_status = st.selectbox(
                    "Update Status",
                    ['applied', 'interview', 'offer', 'rejected', 'no_response', 'withdrawn'],
                    index=['applied', 'interview', 'offer', 'rejected', 'no_response', 'withdrawn'].index(row['status']) if row['status'] in ['applied', 'interview', 'offer', 'rejected', 'no_response', 'withdrawn'] else 0,
                    key=f"status_{row['id']}"
                )

                if st.button("Update", key=f"update_{row['id']}"):
                    try:
                        update_query = "UPDATE applications SET status = %s, updated_at = NOW() WHERE id = %s"
                        db.cursor.execute(update_query, (new_status, row['id']))
                        db.conn.commit()
                        st.success("Status updated!")
                        st.cache_data.clear()
                        st.rerun()
                    except Exception as e:
                        st.error(f"Failed to update: {e}")
    else:
        st.info("No applications match your filters.")

    # Add new application form
    st.divider()
    st.subheader("Add New Application")

    with st.form("new_application"):
        col1, col2 = st.columns(2)

        with col1:
            new_company = st.text_input("Company Name *")
            new_position = st.text_input("Position Title *")

        with col2:
            new_date = st.date_input("Application Date", value=datetime.now().date())
            new_url = st.text_input("Job URL (optional)")

        new_notes = st.text_area("Notes (optional)")

        submitted = st.form_submit_button("Add Application")

        if submitted:
            if not new_company or not new_position:
                st.error("Company and Position are required!")
            else:
                try:
                    # Find or create company
                    company = db.find_company_by_name(new_company)
                    if not company:
                        company_id = db.insert_company(new_company)
                    else:
                        company_id = company['id']

                    # Insert application
                    app_id = db.insert_application(
                        company_id=company_id,
                        position_title=new_position,
                        application_date=new_date,
                        job_url=new_url if new_url else None,
                        notes=new_notes if new_notes else None
                    )

                    st.success(f"Application added successfully! (ID: {app_id})")
                    st.cache_data.clear()
                    st.rerun()
                except Exception as e:
                    st.error(f"Failed to add application: {e}")


def render_analytics_page(analytics: DashboardAnalytics):
    """Render the analytics page"""
    st.title("Analytics")

    # Time period selector
    time_period = st.selectbox(
        "Time Period",
        ["Last 30 Days", "Last 60 Days", "Last 90 Days", "Last 6 Months", "Last Year"],
        index=0
    )

    # Map selection to days
    period_map = {
        "Last 30 Days": 30,
        "Last 60 Days": 60,
        "Last 90 Days": 90,
        "Last 6 Months": 180,
        "Last Year": 365
    }
    days = period_map[time_period]

    # Applications over time
    st.subheader("Applications Over Time")

    apps_over_time = get_cached_applications_over_time(analytics, days)

    if not apps_over_time.empty:
        fig = px.line(
            apps_over_time,
            x='date',
            y='count',
            markers=True
        )
        fig.update_layout(
            paper_bgcolor='rgba(0,0,0,0)',
            plot_bgcolor='rgba(0,0,0,0)',
            font_color='#ffffff',
            xaxis_title="Date",
            yaxis_title="Applications",
            xaxis=dict(gridcolor='#2d3250'),
            yaxis=dict(gridcolor='#2d3250'),
            margin=dict(t=20, b=40, l=40, r=20)
        )
        fig.update_traces(line_color='#3b82f6', marker_color='#3b82f6')
        st.plotly_chart(fig, use_container_width=True)
    else:
        st.info("No data available for the selected period")

    # Two column layout for pie chart and bar chart
    col1, col2 = st.columns(2)

    with col1:
        st.subheader("Status Breakdown")

        status_df = get_cached_status_breakdown(analytics)

        if not status_df.empty:
            color_map = {
                'applied': '#3b82f6',
                'interview': '#8b5cf6',
                'offer': '#10b981',
                'rejected': '#ef4444',
                'no_response': '#6b7280',
                'withdrawn': '#f59e0b'
            }

            fig = px.pie(
                status_df,
                values='count',
                names='status',
                hole=0.4,
                color='status',
                color_discrete_map=color_map
            )
            fig.update_layout(
                paper_bgcolor='rgba(0,0,0,0)',
                plot_bgcolor='rgba(0,0,0,0)',
                font_color='#ffffff',
                showlegend=True,
                legend=dict(
                    orientation="h",
                    yanchor="bottom",
                    y=-0.3,
                    xanchor="center",
                    x=0.5
                ),
                margin=dict(t=20, b=60, l=20, r=20)
            )
            st.plotly_chart(fig, use_container_width=True)
        else:
            st.info("No status data available")

    with col2:
        st.subheader("Top Companies")

        top_companies = get_cached_top_companies(analytics, 10)

        if not top_companies.empty:
            fig = px.bar(
                top_companies,
                x='applications',
                y='company',
                orientation='h'
            )
            fig.update_layout(
                paper_bgcolor='rgba(0,0,0,0)',
                plot_bgcolor='rgba(0,0,0,0)',
                font_color='#ffffff',
                xaxis_title="Applications",
                yaxis_title="",
                xaxis=dict(gridcolor='#2d3250'),
                yaxis=dict(gridcolor='#2d3250'),
                margin=dict(t=20, b=40, l=20, r=20)
            )
            fig.update_traces(marker_color='#8b5cf6')
            st.plotly_chart(fig, use_container_width=True)
        else:
            st.info("No company data available")

    # Response rate trend
    st.subheader("Response Rate Trend")

    months = days // 30 if days >= 30 else 1
    response_trend = get_cached_response_rate_trend(analytics, months)

    if not response_trend.empty:
        response_trend['month'] = pd.to_datetime(response_trend['month'])

        fig = px.line(
            response_trend,
            x='month',
            y='response_rate',
            markers=True
        )
        fig.update_layout(
            paper_bgcolor='rgba(0,0,0,0)',
            plot_bgcolor='rgba(0,0,0,0)',
            font_color='#ffffff',
            xaxis_title="Month",
            yaxis_title="Response Rate (%)",
            xaxis=dict(gridcolor='#2d3250'),
            yaxis=dict(gridcolor='#2d3250', range=[0, 100]),
            margin=dict(t=20, b=40, l=40, r=20)
        )
        fig.update_traces(line_color='#10b981', marker_color='#10b981')
        st.plotly_chart(fig, use_container_width=True)
    else:
        st.info("Not enough data for response rate trend")


def render_settings_page(config: Config, state: StateManager, gmail_auth: GmailAuthenticator, analytics: DashboardAnalytics):
    """Render the settings page"""
    st.title("Settings")

    # Gmail Connection Status
    st.subheader("Gmail Connection")

    is_authenticated = gmail_auth.is_authenticated()

    col1, col2 = st.columns([3, 1])

    with col1:
        if is_authenticated:
            st.success("Gmail is connected and authenticated")
        else:
            st.warning("Gmail is not connected. Run the tracker CLI to authenticate.")

    with col2:
        if st.button("Check Status"):
            st.rerun()

    # Last sync info
    st.subheader("Sync Status")

    last_check = state.get_last_check()

    if last_check:
        st.write(f"**Last sync:** {last_check.strftime('%Y-%m-%d %H:%M:%S')}")
        time_since = datetime.now() - last_check
        hours_since = time_since.total_seconds() / 3600

        if hours_since < 1:
            st.write(f"*{int(time_since.total_seconds() / 60)} minutes ago*")
        elif hours_since < 24:
            st.write(f"*{int(hours_since)} hours ago*")
        else:
            st.write(f"*{int(hours_since / 24)} days ago*")
    else:
        st.write("**Last sync:** Never")

    st.info("To sync new emails, run the Gmail Job Tracker CLI: `python -m gmail_tracker`")

    st.divider()

    # Export Data
    st.subheader("Export Data")

    # Import exporters
    try:
        from .exporter import PDFExporter, CSVExporter, JSONExporter, REPORTLAB_AVAILABLE
        exporters_available = True
    except ImportError:
        exporters_available = False
        REPORTLAB_AVAILABLE = False

    export_tabs = st.tabs(["CSV Export", "PDF Export", "JSON Export"])

    with export_tabs[0]:
        st.write("**Export applications to CSV format**")

        csv_type = st.selectbox(
            "Export Type",
            ["Applications", "Companies", "Full Backup"],
            key="csv_export_type"
        )

        if st.button("Generate CSV", key="csv_btn"):
            try:
                if exporters_available:
                    csv_exporter = CSVExporter()
                    db = st.session_state.dashboard.db

                    if csv_type == "Applications":
                        csv_data = csv_exporter.export_applications(db)
                        filename = f"applications_{datetime.now().strftime('%Y%m%d_%H%M%S')}.csv"
                    elif csv_type == "Companies":
                        csv_data = csv_exporter.export_companies(db)
                        filename = f"companies_{datetime.now().strftime('%Y%m%d_%H%M%S')}.csv"
                    else:
                        csv_data = csv_exporter.export_full_backup(db)
                        filename = f"full_backup_{datetime.now().strftime('%Y%m%d_%H%M%S')}.csv"

                    st.download_button(
                        label="Download CSV",
                        data=csv_data,
                        file_name=filename,
                        mime="text/csv",
                        key="csv_download"
                    )
                    st.success("CSV export ready!")
                else:
                    # Fallback to pandas
                    all_apps = analytics.get_all_applications()
                    if not all_apps.empty:
                        csv_data = all_apps.to_csv(index=False)
                        st.download_button(
                            label="Download CSV",
                            data=csv_data,
                            file_name=f"applications_{datetime.now().strftime('%Y%m%d_%H%M%S')}.csv",
                            mime="text/csv"
                        )
                        st.success("Export ready!")
                    else:
                        st.warning("No data to export")
            except Exception as e:
                st.error(f"CSV export failed: {e}")

    with export_tabs[1]:
        st.write("**Export reports to PDF format**")

        if not REPORTLAB_AVAILABLE:
            st.warning("PDF export requires reportlab. Install with: `pip install reportlab matplotlib`")
        else:
            pdf_type = st.selectbox(
                "Report Type",
                ["Summary Report", "Analytics Report"],
                key="pdf_export_type"
            )

            if pdf_type == "Analytics Report":
                pdf_period = st.selectbox(
                    "Time Period",
                    ["week", "month", "quarter", "year"],
                    index=1,
                    key="pdf_period"
                )
            else:
                pdf_period = "month"

            if st.button("Generate PDF", key="pdf_btn"):
                try:
                    pdf_exporter = PDFExporter()
                    db = st.session_state.dashboard.db

                    with st.spinner("Generating PDF report..."):
                        if pdf_type == "Summary Report":
                            pdf_data = pdf_exporter.generate_summary_report(db)
                            filename = f"summary_report_{datetime.now().strftime('%Y%m%d_%H%M%S')}.pdf"
                        else:
                            pdf_data = pdf_exporter.generate_analytics_report(db, period=pdf_period)
                            filename = f"analytics_report_{datetime.now().strftime('%Y%m%d_%H%M%S')}.pdf"

                    st.download_button(
                        label="Download PDF",
                        data=pdf_data,
                        file_name=filename,
                        mime="application/pdf",
                        key="pdf_download"
                    )
                    st.success("PDF report ready!")
                except Exception as e:
                    st.error(f"PDF export failed: {e}")

    with export_tabs[2]:
        st.write("**Export all data to JSON format (for backup/API)**")

        if st.button("Generate JSON", key="json_btn"):
            try:
                if exporters_available:
                    json_exporter = JSONExporter()
                    db = st.session_state.dashboard.db

                    json_data = json_exporter.export_to_string(db)
                    filename = f"job_tracker_backup_{datetime.now().strftime('%Y%m%d_%H%M%S')}.json"

                    st.download_button(
                        label="Download JSON",
                        data=json_data,
                        file_name=filename,
                        mime="application/json",
                        key="json_download"
                    )
                    st.success("JSON export ready!")
                else:
                    st.error("JSON exporter not available")
            except Exception as e:
                st.error(f"JSON export failed: {e}")

    st.divider()

    # Configuration Info
    st.subheader("Configuration")

    with st.expander("Database Configuration"):
        st.write(f"**Host:** {config.database.get('host', 'localhost')}")
        st.write(f"**Port:** {config.database.get('port', 5432)}")
        st.write(f"**Database:** {config.database.get('database')}")
        st.write(f"**User:** {config.database.get('user')}")

    with st.expander("Gmail Settings"):
        st.write(f"**Check Interval:** {config.gmail.get('check_interval_hours')} hours")
        st.write(f"**Initial Lookback:** {config.gmail.get('initial_lookback_days')} days")
        st.write(f"**Max Emails Per Run:** {config.gmail.get('max_emails_per_run')}")

    with st.expander("LLM Settings"):
        st.write(f"**Provider:** {config.llm.get('provider')}")
        st.write(f"**Model:** {config.llm.get('model')}")

    st.divider()

    # Cache Management
    st.subheader("Cache Management")

    if st.button("Clear Cache"):
        st.cache_data.clear()
        st.success("Cache cleared!")
        st.rerun()

    st.divider()

    # About
    st.subheader("About")
    st.write("**Gmail Job Tracker Dashboard** v1.0.0")
    st.write("A Streamlit-based dashboard for tracking job applications extracted from Gmail.")


def main():
    """Main application entry point"""

    # Initialize session state
    if 'dashboard' not in st.session_state:
        st.session_state.dashboard = Dashboard()
        st.session_state.initialized = False

    dashboard = st.session_state.dashboard

    # Try to initialize if not already done
    if not st.session_state.initialized:
        with st.spinner("Connecting to database..."):
            st.session_state.initialized = dashboard.initialize()

    if not st.session_state.initialized:
        st.error("Failed to initialize dashboard. Please check your configuration.")
        st.write("Make sure the config file exists at: `~/.config/gmail-job-tracker/config.yaml`")
        return

    # Sidebar navigation
    with st.sidebar:
        st.title("Job Tracker")
        st.divider()

        # Navigation
        page = st.radio(
            "Navigation",
            ["Dashboard", "Applications", "Analytics", "Settings"],
            label_visibility="collapsed"
        )

        st.divider()

        # Quick actions
        st.subheader("Quick Actions")

        if st.button("Refresh Data"):
            st.cache_data.clear()
            st.rerun()

        st.divider()

        # Footer info
        stats = get_cached_stats(dashboard.analytics)
        st.caption(f"Total: {stats['total_applications']} applications")
        st.caption(f"Last updated: {datetime.now().strftime('%H:%M:%S')}")

    # Render selected page
    if page == "Dashboard":
        render_dashboard_page(dashboard.analytics)
    elif page == "Applications":
        render_applications_page(dashboard.analytics, dashboard.db)
    elif page == "Analytics":
        render_analytics_page(dashboard.analytics)
    elif page == "Settings":
        render_settings_page(dashboard.config, dashboard.state, dashboard.gmail_auth, dashboard.analytics)


if __name__ == "__main__":
    main()
