"""Export functionality for job application tracking

Provides PDF, CSV, and JSON export capabilities for reports and backups.
"""

import io
import csv
import json
from datetime import datetime, timedelta
from typing import Dict, Any, List, Optional, Tuple
from pathlib import Path

# PDF generation
try:
    from reportlab.lib import colors
    from reportlab.lib.pagesizes import letter, A4
    from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
    from reportlab.lib.units import inch
    from reportlab.platypus import (
        SimpleDocTemplate, Paragraph, Spacer, Table, TableStyle,
        Image, PageBreak, HRFlowable
    )
    from reportlab.lib.enums import TA_CENTER, TA_LEFT, TA_RIGHT
    REPORTLAB_AVAILABLE = True
except ImportError:
    REPORTLAB_AVAILABLE = False

# Chart generation for PDF
try:
    import matplotlib
    matplotlib.use('Agg')  # Non-interactive backend
    import matplotlib.pyplot as plt
    MATPLOTLIB_AVAILABLE = True
except ImportError:
    MATPLOTLIB_AVAILABLE = False

from .database import DatabaseManager
from .analytics import (
    get_summary_stats,
    get_status_breakdown,
    get_top_companies,
    get_applications_over_time,
    get_funnel_stats,
    get_application_velocity,
    get_weekly_summary,
    get_company_response_rates,
    get_best_application_days,
)


class ExporterError(Exception):
    """Raised when export operations fail"""
    pass


class PDFExporter:
    """Generates PDF reports for job applications"""

    # Color scheme
    COLORS = {
        'primary': colors.HexColor('#3b82f6'),
        'secondary': colors.HexColor('#8b5cf6'),
        'success': colors.HexColor('#10b981'),
        'danger': colors.HexColor('#ef4444'),
        'warning': colors.HexColor('#f59e0b'),
        'muted': colors.HexColor('#6b7280'),
        'dark': colors.HexColor('#1f2937'),
        'light': colors.HexColor('#f3f4f6'),
    }

    STATUS_COLORS = {
        'applied': colors.HexColor('#3b82f6'),
        'interview': colors.HexColor('#8b5cf6'),
        'interviewing': colors.HexColor('#8b5cf6'),
        'offer': colors.HexColor('#10b981'),
        'rejected': colors.HexColor('#ef4444'),
        'no_response': colors.HexColor('#6b7280'),
        'withdrawn': colors.HexColor('#f59e0b'),
    }

    def __init__(self):
        """Initialize PDF exporter"""
        if not REPORTLAB_AVAILABLE:
            raise ExporterError(
                "reportlab is required for PDF export. "
                "Install with: pip install reportlab"
            )
        self.styles = getSampleStyleSheet()
        self._setup_custom_styles()

    def _setup_custom_styles(self):
        """Setup custom paragraph styles"""
        self.styles.add(ParagraphStyle(
            name='Title_Custom',
            parent=self.styles['Title'],
            fontSize=24,
            textColor=self.COLORS['dark'],
            spaceAfter=20,
        ))
        self.styles.add(ParagraphStyle(
            name='Heading1_Custom',
            parent=self.styles['Heading1'],
            fontSize=18,
            textColor=self.COLORS['primary'],
            spaceBefore=20,
            spaceAfter=10,
        ))
        self.styles.add(ParagraphStyle(
            name='Heading2_Custom',
            parent=self.styles['Heading2'],
            fontSize=14,
            textColor=self.COLORS['dark'],
            spaceBefore=15,
            spaceAfter=8,
        ))
        self.styles.add(ParagraphStyle(
            name='Body_Custom',
            parent=self.styles['Normal'],
            fontSize=10,
            textColor=self.COLORS['dark'],
            spaceAfter=6,
        ))
        self.styles.add(ParagraphStyle(
            name='Caption',
            parent=self.styles['Normal'],
            fontSize=8,
            textColor=self.COLORS['muted'],
            alignment=TA_CENTER,
        ))
        self.styles.add(ParagraphStyle(
            name='Metric_Value',
            parent=self.styles['Normal'],
            fontSize=24,
            textColor=self.COLORS['primary'],
            alignment=TA_CENTER,
        ))
        self.styles.add(ParagraphStyle(
            name='Metric_Label',
            parent=self.styles['Normal'],
            fontSize=10,
            textColor=self.COLORS['muted'],
            alignment=TA_CENTER,
        ))

    def _add_header(self, elements: List, title: str, date_range: Optional[Tuple[datetime, datetime]] = None):
        """Add report header"""
        elements.append(Paragraph(title, self.styles['Title_Custom']))

        if date_range:
            start, end = date_range
            date_str = f"Report Period: {start.strftime('%B %d, %Y')} - {end.strftime('%B %d, %Y')}"
        else:
            date_str = f"Generated: {datetime.now().strftime('%B %d, %Y at %I:%M %p')}"

        elements.append(Paragraph(date_str, self.styles['Caption']))
        elements.append(Spacer(1, 20))
        elements.append(HRFlowable(width="100%", thickness=1, color=self.COLORS['light']))
        elements.append(Spacer(1, 20))

    def _add_footer(self, canvas, doc):
        """Add footer with page numbers"""
        canvas.saveState()
        canvas.setFont('Helvetica', 8)
        canvas.setFillColor(self.COLORS['muted'])
        page_num = f"Page {doc.page}"
        canvas.drawRightString(doc.width + doc.leftMargin, 0.5 * inch, page_num)
        canvas.drawString(doc.leftMargin, 0.5 * inch, "Gmail Job Tracker Report")
        canvas.restoreState()

    def _create_metric_table(self, metrics: List[Tuple[str, str]]) -> Table:
        """Create a table of metric cards"""
        # Build rows of 4 metrics each
        rows = []
        current_row = []

        for value, label in metrics:
            cell_content = [
                Paragraph(str(value), self.styles['Metric_Value']),
                Paragraph(label, self.styles['Metric_Label']),
            ]
            current_row.append(cell_content)

            if len(current_row) == 4:
                rows.append(current_row)
                current_row = []

        if current_row:
            # Pad with empty cells
            while len(current_row) < 4:
                current_row.append(['', ''])
            rows.append(current_row)

        if not rows:
            return Spacer(1, 0)

        table = Table(rows, colWidths=[1.5 * inch] * 4)
        table.setStyle(TableStyle([
            ('ALIGN', (0, 0), (-1, -1), 'CENTER'),
            ('VALIGN', (0, 0), (-1, -1), 'MIDDLE'),
            ('BOX', (0, 0), (-1, -1), 1, self.COLORS['light']),
            ('INNERGRID', (0, 0), (-1, -1), 0.5, self.COLORS['light']),
            ('TOPPADDING', (0, 0), (-1, -1), 15),
            ('BOTTOMPADDING', (0, 0), (-1, -1), 15),
        ]))

        return table

    def _create_chart_image(self, chart_func, width: float = 6 * inch, height: float = 3 * inch) -> Optional[Image]:
        """Create a chart image from matplotlib figure"""
        if not MATPLOTLIB_AVAILABLE:
            return None

        try:
            fig = chart_func()
            if fig is None:
                return None

            buf = io.BytesIO()
            fig.savefig(buf, format='png', dpi=150, bbox_inches='tight',
                       facecolor='white', edgecolor='none')
            plt.close(fig)
            buf.seek(0)

            return Image(buf, width=width, height=height)
        except Exception:
            return None

    def _create_status_pie_chart(self, status_data: Dict[str, int]):
        """Create status breakdown pie chart"""
        if not status_data:
            return None

        labels = list(status_data.keys())
        sizes = list(status_data.values())

        # Map colors
        chart_colors = [
            self.STATUS_COLORS.get(s.lower(), self.COLORS['muted']).hexval()[2:]
            for s in labels
        ]
        chart_colors = ['#' + c for c in chart_colors]

        fig, ax = plt.subplots(figsize=(6, 4))
        wedges, texts, autotexts = ax.pie(
            sizes, labels=labels, autopct='%1.0f%%',
            colors=chart_colors, startangle=90
        )
        ax.set_title('Application Status Breakdown', fontsize=12, fontweight='bold')

        # Style the text
        for text in texts:
            text.set_fontsize(9)
        for autotext in autotexts:
            autotext.set_fontsize(8)
            autotext.set_color('white')

        return fig

    def _create_timeline_chart(self, timeline_data: List[Dict[str, Any]]):
        """Create applications over time line chart"""
        if not timeline_data:
            return None

        dates = [d['date'] for d in timeline_data]
        counts = [d['count'] for d in timeline_data]

        fig, ax = plt.subplots(figsize=(6, 3))
        ax.plot(dates, counts, color='#3b82f6', marker='o', linewidth=2, markersize=4)
        ax.fill_between(dates, counts, alpha=0.2, color='#3b82f6')
        ax.set_xlabel('Date', fontsize=9)
        ax.set_ylabel('Applications', fontsize=9)
        ax.set_title('Application Activity Over Time', fontsize=12, fontweight='bold')
        ax.tick_params(axis='x', rotation=45, labelsize=7)
        ax.tick_params(axis='y', labelsize=8)
        ax.grid(True, alpha=0.3)
        fig.tight_layout()

        return fig

    def _create_funnel_chart(self, funnel_data: Dict[str, Any]):
        """Create conversion funnel bar chart"""
        if not funnel_data:
            return None

        stages = ['Applied', 'Responded', 'Interviewed', 'Offered']
        values = [
            funnel_data.get('applied', 0),
            funnel_data.get('responded', 0),
            funnel_data.get('interviewed', 0),
            funnel_data.get('offered', 0),
        ]

        fig, ax = plt.subplots(figsize=(6, 3))
        colors_list = ['#3b82f6', '#8b5cf6', '#10b981', '#f59e0b']
        bars = ax.barh(stages, values, color=colors_list)

        # Add value labels
        for bar, val in zip(bars, values):
            ax.text(bar.get_width() + 0.5, bar.get_y() + bar.get_height()/2,
                   str(val), va='center', fontsize=9)

        ax.set_xlabel('Count', fontsize=9)
        ax.set_title('Application Funnel', fontsize=12, fontweight='bold')
        ax.tick_params(axis='both', labelsize=8)
        ax.invert_yaxis()
        fig.tight_layout()

        return fig

    def generate_summary_report(self, db: DatabaseManager,
                               date_range: Optional[Tuple[datetime, datetime]] = None) -> bytes:
        """Generate PDF summary report

        Args:
            db: DatabaseManager instance with active connection
            date_range: Optional tuple of (start_date, end_date) for filtering

        Returns:
            PDF content as bytes
        """
        buffer = io.BytesIO()
        doc = SimpleDocTemplate(
            buffer,
            pagesize=letter,
            rightMargin=0.75 * inch,
            leftMargin=0.75 * inch,
            topMargin=0.75 * inch,
            bottomMargin=0.75 * inch,
        )

        elements = []

        # Header
        self._add_header(elements, "Job Application Summary Report", date_range)

        # Get data
        try:
            stats = get_summary_stats(db)
            status_breakdown = get_status_breakdown(db)
            top_companies = get_top_companies(db, limit=10)
            timeline = get_applications_over_time(db, period='week')
        except Exception as e:
            raise ExporterError(f"Failed to fetch data for report: {e}")

        # Summary Stats Section
        elements.append(Paragraph("Summary Statistics", self.styles['Heading1_Custom']))

        metrics = [
            (str(stats['total_applications']), "Total Applications"),
            (str(stats['total_companies']), "Companies Applied"),
            (f"{stats['response_rate']}%", "Response Rate"),
            (str(stats['interviews_scheduled']), "Interviews"),
            (str(stats['offers_received']), "Offers"),
            (str(stats['rejection_count']), "Rejections"),
            (str(stats['pending_count']), "Pending"),
            (f"{stats['average_response_time_days'] or 'N/A'}", "Avg Response (days)"),
        ]

        elements.append(self._create_metric_table(metrics))
        elements.append(Spacer(1, 20))

        # Status Breakdown Chart
        if status_breakdown and MATPLOTLIB_AVAILABLE:
            elements.append(Paragraph("Status Breakdown", self.styles['Heading2_Custom']))
            chart = self._create_chart_image(
                lambda: self._create_status_pie_chart(status_breakdown),
                width=5 * inch, height=3 * inch
            )
            if chart:
                elements.append(chart)
            elements.append(Spacer(1, 15))

        # Top Companies Table
        if top_companies:
            elements.append(Paragraph("Top Companies", self.styles['Heading2_Custom']))

            table_data = [['Company', 'Applications']]
            for company in top_companies[:10]:
                table_data.append([
                    company['company_name'],
                    str(company['application_count'])
                ])

            table = Table(table_data, colWidths=[4 * inch, 1.5 * inch])
            table.setStyle(TableStyle([
                ('BACKGROUND', (0, 0), (-1, 0), self.COLORS['primary']),
                ('TEXTCOLOR', (0, 0), (-1, 0), colors.white),
                ('FONTNAME', (0, 0), (-1, 0), 'Helvetica-Bold'),
                ('FONTSIZE', (0, 0), (-1, 0), 10),
                ('ALIGN', (0, 0), (-1, -1), 'LEFT'),
                ('ALIGN', (1, 0), (1, -1), 'CENTER'),
                ('BOTTOMPADDING', (0, 0), (-1, 0), 10),
                ('TOPPADDING', (0, 0), (-1, 0), 10),
                ('BACKGROUND', (0, 1), (-1, -1), colors.white),
                ('TEXTCOLOR', (0, 1), (-1, -1), self.COLORS['dark']),
                ('FONTSIZE', (0, 1), (-1, -1), 9),
                ('BOTTOMPADDING', (0, 1), (-1, -1), 6),
                ('TOPPADDING', (0, 1), (-1, -1), 6),
                ('BOX', (0, 0), (-1, -1), 1, self.COLORS['light']),
                ('INNERGRID', (0, 0), (-1, -1), 0.5, self.COLORS['light']),
                ('ROWBACKGROUNDS', (0, 1), (-1, -1), [colors.white, self.COLORS['light']]),
            ]))
            elements.append(table)
            elements.append(Spacer(1, 15))

        # Timeline Chart
        if timeline and MATPLOTLIB_AVAILABLE:
            elements.append(Paragraph("Application Activity", self.styles['Heading2_Custom']))
            chart = self._create_chart_image(
                lambda: self._create_timeline_chart(timeline),
                width=6 * inch, height=2.5 * inch
            )
            if chart:
                elements.append(chart)

        # Build PDF
        doc.build(elements, onFirstPage=self._add_footer, onLaterPages=self._add_footer)

        buffer.seek(0)
        return buffer.getvalue()

    def generate_application_detail(self, db: DatabaseManager, app_id: int) -> bytes:
        """Generate PDF for single application

        Args:
            db: DatabaseManager instance with active connection
            app_id: Application ID

        Returns:
            PDF content as bytes
        """
        buffer = io.BytesIO()
        doc = SimpleDocTemplate(
            buffer,
            pagesize=letter,
            rightMargin=0.75 * inch,
            leftMargin=0.75 * inch,
            topMargin=0.75 * inch,
            bottomMargin=0.75 * inch,
        )

        elements = []

        # Get application data
        try:
            db.cursor.execute("""
                SELECT
                    a.id,
                    c.name as company_name,
                    c.website as company_website,
                    a.position_title,
                    a.status,
                    a.application_date,
                    a.response_date,
                    a.job_url,
                    a.notes,
                    a.created_at,
                    a.updated_at
                FROM applications a
                JOIN companies c ON a.company_id = c.id
                WHERE a.id = %s
            """, (app_id,))
            app = db.cursor.fetchone()

            if not app:
                raise ExporterError(f"Application {app_id} not found")

        except Exception as e:
            raise ExporterError(f"Failed to fetch application data: {e}")

        # Header
        elements.append(Paragraph("Application Detail", self.styles['Title_Custom']))
        elements.append(Paragraph(
            f"Generated: {datetime.now().strftime('%B %d, %Y at %I:%M %p')}",
            self.styles['Caption']
        ))
        elements.append(Spacer(1, 20))

        # Company Info
        elements.append(Paragraph("Company Information", self.styles['Heading1_Custom']))

        company_data = [
            ['Company', app['company_name']],
            ['Website', app['company_website'] or 'N/A'],
        ]
        company_table = Table(company_data, colWidths=[1.5 * inch, 4.5 * inch])
        company_table.setStyle(TableStyle([
            ('FONTNAME', (0, 0), (0, -1), 'Helvetica-Bold'),
            ('FONTSIZE', (0, 0), (-1, -1), 10),
            ('TEXTCOLOR', (0, 0), (-1, -1), self.COLORS['dark']),
            ('BOTTOMPADDING', (0, 0), (-1, -1), 8),
            ('TOPPADDING', (0, 0), (-1, -1), 8),
        ]))
        elements.append(company_table)
        elements.append(Spacer(1, 15))

        # Position Details
        elements.append(Paragraph("Position Details", self.styles['Heading1_Custom']))

        status_color = self.STATUS_COLORS.get(app['status'], self.COLORS['muted'])
        position_data = [
            ['Position', app['position_title']],
            ['Status', app['status'].replace('_', ' ').title()],
            ['Applied Date', app['application_date'].strftime('%B %d, %Y') if app['application_date'] else 'N/A'],
            ['Response Date', app['response_date'].strftime('%B %d, %Y') if app.get('response_date') else 'Pending'],
            ['Job URL', app['job_url'] or 'N/A'],
        ]
        position_table = Table(position_data, colWidths=[1.5 * inch, 4.5 * inch])
        position_table.setStyle(TableStyle([
            ('FONTNAME', (0, 0), (0, -1), 'Helvetica-Bold'),
            ('FONTSIZE', (0, 0), (-1, -1), 10),
            ('TEXTCOLOR', (0, 0), (-1, -1), self.COLORS['dark']),
            ('BOTTOMPADDING', (0, 0), (-1, -1), 8),
            ('TOPPADDING', (0, 0), (-1, -1), 8),
        ]))
        elements.append(position_table)
        elements.append(Spacer(1, 15))

        # Notes
        if app.get('notes'):
            elements.append(Paragraph("Notes", self.styles['Heading1_Custom']))
            elements.append(Paragraph(app['notes'], self.styles['Body_Custom']))
            elements.append(Spacer(1, 15))

        # Timeline
        elements.append(Paragraph("Timeline", self.styles['Heading1_Custom']))

        timeline_data = []
        if app['created_at']:
            timeline_data.append([
                app['created_at'].strftime('%Y-%m-%d %H:%M'),
                'Application tracked'
            ])
        if app['application_date']:
            timeline_data.append([
                app['application_date'].strftime('%Y-%m-%d'),
                'Applied for position'
            ])
        if app.get('response_date'):
            timeline_data.append([
                app['response_date'].strftime('%Y-%m-%d'),
                f'Response received ({app["status"]})'
            ])
        if app['updated_at'] and app['updated_at'] != app['created_at']:
            timeline_data.append([
                app['updated_at'].strftime('%Y-%m-%d %H:%M'),
                'Last updated'
            ])

        if timeline_data:
            timeline_table = Table(timeline_data, colWidths=[1.5 * inch, 4.5 * inch])
            timeline_table.setStyle(TableStyle([
                ('FONTSIZE', (0, 0), (-1, -1), 9),
                ('TEXTCOLOR', (0, 0), (0, -1), self.COLORS['muted']),
                ('TEXTCOLOR', (1, 0), (1, -1), self.COLORS['dark']),
                ('BOTTOMPADDING', (0, 0), (-1, -1), 6),
                ('TOPPADDING', (0, 0), (-1, -1), 6),
                ('LEFTPADDING', (0, 0), (0, -1), 0),
            ]))
            elements.append(timeline_table)

        # Build PDF
        doc.build(elements, onFirstPage=self._add_footer, onLaterPages=self._add_footer)

        buffer.seek(0)
        return buffer.getvalue()

    def generate_analytics_report(self, db: DatabaseManager, period: str = 'month') -> bytes:
        """Generate analytics PDF with charts

        Args:
            db: DatabaseManager instance with active connection
            period: Time period - 'week', 'month', 'quarter', 'year'

        Returns:
            PDF content as bytes
        """
        buffer = io.BytesIO()
        doc = SimpleDocTemplate(
            buffer,
            pagesize=letter,
            rightMargin=0.75 * inch,
            leftMargin=0.75 * inch,
            topMargin=0.75 * inch,
            bottomMargin=0.75 * inch,
        )

        elements = []

        # Calculate date range
        period_days = {
            'week': 7,
            'month': 30,
            'quarter': 90,
            'year': 365,
        }
        days = period_days.get(period, 30)
        end_date = datetime.now()
        start_date = end_date - timedelta(days=days)

        # Header
        self._add_header(elements, "Analytics Report", (start_date, end_date))

        # Get data
        try:
            stats = get_summary_stats(db)
            funnel = get_funnel_stats(db)
            velocity = get_application_velocity(db, days=days)
            weekly_summary = get_weekly_summary(db)
            response_rates = get_company_response_rates(db)
            best_days = get_best_application_days(db)
            timeline = get_applications_over_time(db, period='day' if days <= 30 else 'week')
        except Exception as e:
            raise ExporterError(f"Failed to fetch analytics data: {e}")

        # Key Metrics
        elements.append(Paragraph("Key Metrics", self.styles['Heading1_Custom']))

        metrics = [
            (f"{velocity}", "Apps/Day (avg)"),
            (f"{stats['response_rate']}%", "Response Rate"),
            (f"{funnel['conversion_rates']['interview_rate']}%", "Interview Rate"),
            (f"{funnel['conversion_rates']['offer_rate']}%", "Offer Rate"),
        ]
        elements.append(self._create_metric_table(metrics))
        elements.append(Spacer(1, 20))

        # Weekly Performance
        elements.append(Paragraph("This Week's Performance", self.styles['Heading2_Custom']))

        weekly_metrics = [
            (str(weekly_summary['applications_submitted']), "Applications"),
            (str(weekly_summary['responses_received']), "Responses"),
            (str(weekly_summary['interviews_scheduled']), "Interviews"),
            (f"{weekly_summary['comparison']['week_over_week_change']:+.0f}%", "vs Last Week"),
        ]
        elements.append(self._create_metric_table(weekly_metrics))
        elements.append(Spacer(1, 20))

        # Funnel Chart
        if MATPLOTLIB_AVAILABLE:
            elements.append(Paragraph("Application Funnel", self.styles['Heading2_Custom']))
            chart = self._create_chart_image(
                lambda: self._create_funnel_chart(funnel),
                width=5 * inch, height=2.5 * inch
            )
            if chart:
                elements.append(chart)
            elements.append(Spacer(1, 15))

        # Timeline Chart
        if timeline and MATPLOTLIB_AVAILABLE:
            elements.append(Paragraph("Activity Timeline", self.styles['Heading2_Custom']))
            chart = self._create_chart_image(
                lambda: self._create_timeline_chart(timeline),
                width=6 * inch, height=2.5 * inch
            )
            if chart:
                elements.append(chart)
            elements.append(Spacer(1, 15))

        # Best Days Analysis
        if best_days:
            elements.append(PageBreak())
            elements.append(Paragraph("Best Days to Apply", self.styles['Heading1_Custom']))
            elements.append(Paragraph(
                "Analysis of response rates by day of week",
                self.styles['Body_Custom']
            ))
            elements.append(Spacer(1, 10))

            day_data = [['Day', 'Applications', 'Responses', 'Response Rate']]
            for day, data in best_days.items():
                day_data.append([
                    day,
                    str(data['total_applications']),
                    str(data['responses_received']),
                    f"{data['response_rate']}%"
                ])

            day_table = Table(day_data, colWidths=[1.5 * inch, 1.2 * inch, 1.2 * inch, 1.2 * inch])
            day_table.setStyle(TableStyle([
                ('BACKGROUND', (0, 0), (-1, 0), self.COLORS['primary']),
                ('TEXTCOLOR', (0, 0), (-1, 0), colors.white),
                ('FONTNAME', (0, 0), (-1, 0), 'Helvetica-Bold'),
                ('FONTSIZE', (0, 0), (-1, 0), 10),
                ('ALIGN', (1, 0), (-1, -1), 'CENTER'),
                ('BOTTOMPADDING', (0, 0), (-1, 0), 10),
                ('TOPPADDING', (0, 0), (-1, 0), 10),
                ('BACKGROUND', (0, 1), (-1, -1), colors.white),
                ('TEXTCOLOR', (0, 1), (-1, -1), self.COLORS['dark']),
                ('FONTSIZE', (0, 1), (-1, -1), 9),
                ('BOTTOMPADDING', (0, 1), (-1, -1), 6),
                ('TOPPADDING', (0, 1), (-1, -1), 6),
                ('BOX', (0, 0), (-1, -1), 1, self.COLORS['light']),
                ('INNERGRID', (0, 0), (-1, -1), 0.5, self.COLORS['light']),
            ]))
            elements.append(day_table)
            elements.append(Spacer(1, 20))

        # Company Response Rates
        if response_rates:
            elements.append(Paragraph("Company Response Rates", self.styles['Heading1_Custom']))

            # Sort by response rate and take top 15
            sorted_rates = sorted(
                response_rates,
                key=lambda x: x['response_rate'],
                reverse=True
            )[:15]

            rate_data = [['Company', 'Applications', 'Responses', 'Rate']]
            for company in sorted_rates:
                rate_data.append([
                    company['company_name'][:30],  # Truncate long names
                    str(company['total_applications']),
                    str(company['responses_received']),
                    f"{company['response_rate']}%"
                ])

            rate_table = Table(rate_data, colWidths=[2.5 * inch, 1 * inch, 1 * inch, 0.8 * inch])
            rate_table.setStyle(TableStyle([
                ('BACKGROUND', (0, 0), (-1, 0), self.COLORS['secondary']),
                ('TEXTCOLOR', (0, 0), (-1, 0), colors.white),
                ('FONTNAME', (0, 0), (-1, 0), 'Helvetica-Bold'),
                ('FONTSIZE', (0, 0), (-1, 0), 9),
                ('ALIGN', (1, 0), (-1, -1), 'CENTER'),
                ('BOTTOMPADDING', (0, 0), (-1, 0), 8),
                ('TOPPADDING', (0, 0), (-1, 0), 8),
                ('BACKGROUND', (0, 1), (-1, -1), colors.white),
                ('TEXTCOLOR', (0, 1), (-1, -1), self.COLORS['dark']),
                ('FONTSIZE', (0, 1), (-1, -1), 8),
                ('BOTTOMPADDING', (0, 1), (-1, -1), 5),
                ('TOPPADDING', (0, 1), (-1, -1), 5),
                ('BOX', (0, 0), (-1, -1), 1, self.COLORS['light']),
                ('INNERGRID', (0, 0), (-1, -1), 0.5, self.COLORS['light']),
                ('ROWBACKGROUNDS', (0, 1), (-1, -1), [colors.white, self.COLORS['light']]),
            ]))
            elements.append(rate_table)
            elements.append(Spacer(1, 20))

        # Recommendations
        elements.append(Paragraph("Recommendations", self.styles['Heading1_Custom']))

        recommendations = self._generate_recommendations(stats, funnel, velocity, best_days)
        for i, rec in enumerate(recommendations, 1):
            elements.append(Paragraph(f"{i}. {rec}", self.styles['Body_Custom']))

        # Build PDF
        doc.build(elements, onFirstPage=self._add_footer, onLaterPages=self._add_footer)

        buffer.seek(0)
        return buffer.getvalue()

    def _generate_recommendations(self, stats: Dict, funnel: Dict,
                                  velocity: float, best_days: Dict) -> List[str]:
        """Generate recommendations based on analytics"""
        recommendations = []

        # Velocity recommendations
        if velocity < 1:
            recommendations.append(
                "Consider increasing your application volume. "
                f"Current rate: {velocity} apps/day. Target: 2-5 per day."
            )
        elif velocity > 10:
            recommendations.append(
                "High application volume detected. Consider focusing on quality over quantity "
                "and tailoring applications more specifically."
            )

        # Response rate recommendations
        if stats['response_rate'] < 10:
            recommendations.append(
                "Response rate is below average. Consider revising your resume and cover letter, "
                "or targeting positions that better match your experience."
            )
        elif stats['response_rate'] > 30:
            recommendations.append(
                "Excellent response rate! Your application materials are working well. "
                "Keep refining and targeting appropriate positions."
            )

        # Funnel recommendations
        if funnel['conversion_rates']['interview_rate'] < 20:
            recommendations.append(
                "Interview conversion is low. Practice phone screening responses and "
                "ensure your resume highlights relevant achievements."
            )

        if funnel['conversion_rates']['offer_rate'] < 10 and funnel['interviewed'] > 5:
            recommendations.append(
                "Offer rate after interviews is low. Consider practicing interview skills "
                "or researching companies more thoroughly before interviews."
            )

        # Best day recommendations
        if best_days:
            sorted_days = sorted(
                best_days.items(),
                key=lambda x: x[1]['response_rate'],
                reverse=True
            )
            if sorted_days and sorted_days[0][1]['response_rate'] > 0:
                best_day = sorted_days[0][0]
                recommendations.append(
                    f"{best_day} shows the highest response rate. "
                    "Consider prioritizing applications on this day."
                )

        if not recommendations:
            recommendations.append(
                "Keep up the good work! Continue tracking your applications and "
                "refining your approach based on what works best."
            )

        return recommendations


class CSVExporter:
    """Exports job application data to CSV format"""

    def __init__(self):
        """Initialize CSV exporter"""
        pass

    def export_applications(self, db: DatabaseManager,
                           filters: Optional[Dict[str, Any]] = None) -> str:
        """Export applications to CSV

        Args:
            db: DatabaseManager instance with active connection
            filters: Optional filters (status, date_from, date_to, company)

        Returns:
            CSV content as string
        """
        try:
            query = """
                SELECT
                    a.id,
                    c.name as company,
                    a.position_title as position,
                    a.status,
                    a.application_date,
                    a.response_date,
                    a.job_url,
                    a.notes,
                    a.created_at,
                    a.updated_at
                FROM applications a
                JOIN companies c ON a.company_id = c.id
                WHERE 1=1
            """
            params = []

            if filters:
                if filters.get('status'):
                    query += " AND a.status = %s"
                    params.append(filters['status'])
                if filters.get('date_from'):
                    query += " AND a.application_date >= %s"
                    params.append(filters['date_from'])
                if filters.get('date_to'):
                    query += " AND a.application_date <= %s"
                    params.append(filters['date_to'])
                if filters.get('company'):
                    query += " AND LOWER(c.name) LIKE %s"
                    params.append(f"%{filters['company'].lower()}%")

            query += " ORDER BY a.application_date DESC, a.id DESC"

            db.cursor.execute(query, params if params else None)
            results = db.cursor.fetchall()

            if not results:
                return "id,company,position,status,application_date,response_date,job_url,notes,created_at,updated_at\n"

            # Write to CSV string
            output = io.StringIO()
            writer = csv.writer(output)

            # Header
            columns = ['id', 'company', 'position', 'status', 'application_date',
                      'response_date', 'job_url', 'notes', 'created_at', 'updated_at']
            writer.writerow(columns)

            # Data rows
            for row in results:
                writer.writerow([
                    row['id'],
                    row['company'],
                    row['position'],
                    row['status'],
                    row['application_date'].isoformat() if row['application_date'] else '',
                    row['response_date'].isoformat() if row.get('response_date') else '',
                    row['job_url'] or '',
                    row['notes'] or '',
                    row['created_at'].isoformat() if row['created_at'] else '',
                    row['updated_at'].isoformat() if row['updated_at'] else '',
                ])

            return output.getvalue()

        except Exception as e:
            raise ExporterError(f"Failed to export applications: {e}")

    def export_companies(self, db: DatabaseManager) -> str:
        """Export companies to CSV

        Args:
            db: DatabaseManager instance with active connection

        Returns:
            CSV content as string
        """
        try:
            db.cursor.execute("""
                SELECT
                    c.id,
                    c.name,
                    c.website,
                    c.description,
                    COUNT(a.id) as application_count,
                    c.created_at,
                    c.updated_at
                FROM companies c
                LEFT JOIN applications a ON c.id = a.company_id
                GROUP BY c.id, c.name, c.website, c.description, c.created_at, c.updated_at
                ORDER BY c.name
            """)
            results = db.cursor.fetchall()

            if not results:
                return "id,name,website,description,application_count,created_at,updated_at\n"

            output = io.StringIO()
            writer = csv.writer(output)

            # Header
            writer.writerow(['id', 'name', 'website', 'description',
                           'application_count', 'created_at', 'updated_at'])

            # Data rows
            for row in results:
                writer.writerow([
                    row['id'],
                    row['name'],
                    row['website'] or '',
                    row['description'] or '',
                    row['application_count'],
                    row['created_at'].isoformat() if row['created_at'] else '',
                    row['updated_at'].isoformat() if row['updated_at'] else '',
                ])

            return output.getvalue()

        except Exception as e:
            raise ExporterError(f"Failed to export companies: {e}")

    def export_full_backup(self, db: DatabaseManager) -> str:
        """Export all data for backup (applications with full details)

        Args:
            db: DatabaseManager instance with active connection

        Returns:
            CSV content as string
        """
        try:
            db.cursor.execute("""
                SELECT
                    a.id as application_id,
                    c.id as company_id,
                    c.name as company_name,
                    c.website as company_website,
                    c.description as company_description,
                    a.position_title,
                    a.status,
                    a.application_date,
                    a.response_date,
                    a.job_url,
                    a.notes,
                    a.created_at as application_created,
                    a.updated_at as application_updated,
                    c.created_at as company_created,
                    c.updated_at as company_updated
                FROM applications a
                JOIN companies c ON a.company_id = c.id
                ORDER BY a.application_date DESC, a.id DESC
            """)
            results = db.cursor.fetchall()

            output = io.StringIO()
            writer = csv.writer(output)

            # Header
            columns = [
                'application_id', 'company_id', 'company_name', 'company_website',
                'company_description', 'position_title', 'status', 'application_date',
                'response_date', 'job_url', 'notes', 'application_created',
                'application_updated', 'company_created', 'company_updated'
            ]
            writer.writerow(columns)

            # Data rows
            for row in results:
                writer.writerow([
                    row['application_id'],
                    row['company_id'],
                    row['company_name'],
                    row['company_website'] or '',
                    row['company_description'] or '',
                    row['position_title'],
                    row['status'],
                    row['application_date'].isoformat() if row['application_date'] else '',
                    row['response_date'].isoformat() if row.get('response_date') else '',
                    row['job_url'] or '',
                    row['notes'] or '',
                    row['application_created'].isoformat() if row['application_created'] else '',
                    row['application_updated'].isoformat() if row['application_updated'] else '',
                    row['company_created'].isoformat() if row['company_created'] else '',
                    row['company_updated'].isoformat() if row['company_updated'] else '',
                ])

            return output.getvalue()

        except Exception as e:
            raise ExporterError(f"Failed to export backup: {e}")


class JSONExporter:
    """Exports job application data to JSON format"""

    def __init__(self):
        """Initialize JSON exporter"""
        pass

    def export_all(self, db: DatabaseManager) -> Dict[str, Any]:
        """Export all data as JSON (for API/backup)

        Args:
            db: DatabaseManager instance with active connection

        Returns:
            Dictionary with all data
        """
        try:
            # Get companies
            db.cursor.execute("""
                SELECT id, name, website, description, created_at, updated_at
                FROM companies
                ORDER BY name
            """)
            companies = []
            for row in db.cursor.fetchall():
                companies.append({
                    'id': row['id'],
                    'name': row['name'],
                    'website': row['website'],
                    'description': row['description'],
                    'created_at': row['created_at'].isoformat() if row['created_at'] else None,
                    'updated_at': row['updated_at'].isoformat() if row['updated_at'] else None,
                })

            # Get applications
            db.cursor.execute("""
                SELECT
                    a.id,
                    a.company_id,
                    c.name as company_name,
                    a.position_title,
                    a.status,
                    a.application_date,
                    a.response_date,
                    a.job_url,
                    a.notes,
                    a.created_at,
                    a.updated_at
                FROM applications a
                JOIN companies c ON a.company_id = c.id
                ORDER BY a.application_date DESC
            """)
            applications = []
            for row in db.cursor.fetchall():
                applications.append({
                    'id': row['id'],
                    'company_id': row['company_id'],
                    'company_name': row['company_name'],
                    'position_title': row['position_title'],
                    'status': row['status'],
                    'application_date': row['application_date'].isoformat() if row['application_date'] else None,
                    'response_date': row['response_date'].isoformat() if row.get('response_date') else None,
                    'job_url': row['job_url'],
                    'notes': row['notes'],
                    'created_at': row['created_at'].isoformat() if row['created_at'] else None,
                    'updated_at': row['updated_at'].isoformat() if row['updated_at'] else None,
                })

            # Get summary stats
            stats = get_summary_stats(db)

            return {
                'export_date': datetime.now().isoformat(),
                'version': '1.0',
                'summary': stats,
                'companies': companies,
                'applications': applications,
            }

        except Exception as e:
            raise ExporterError(f"Failed to export data: {e}")

    def export_to_string(self, db: DatabaseManager, indent: int = 2) -> str:
        """Export all data as JSON string

        Args:
            db: DatabaseManager instance with active connection
            indent: JSON indentation level

        Returns:
            JSON string
        """
        data = self.export_all(db)
        return json.dumps(data, indent=indent, default=str)
