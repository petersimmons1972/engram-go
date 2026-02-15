#!/usr/bin/env python3
"""
Gmail Job Application Tracker

Monitors Gmail for job application confirmation emails and populates
the job-search-system PostgreSQL database.
"""

import sys
import argparse
from pathlib import Path

# Add gmail_tracker to path
sys.path.insert(0, str(Path(__file__).parent))

from gmail_tracker.config import Config
from gmail_tracker.app import GmailJobTracker
from gmail_tracker.gmail_auth import GmailAuthenticator
from gmail_tracker.logger import Logger
from gmail_tracker.database import DatabaseManager

# Version
__version__ = "1.0.0"

def cmd_run(args):
    """Run the job tracker"""
    config = Config()
    app = GmailJobTracker(config)
    return app.run(dry_run=args.dry_run)

def cmd_setup(args):
    """Run initial OAuth setup"""
    logger = Logger()
    logger.info("Starting Gmail OAuth setup")

    auth = GmailAuthenticator()
    try:
        if args.remote:
            logger.info("Using remote/console authentication flow")
            service = auth.authenticate_console()
        else:
            service = auth.authenticate(interactive=True)
        logger.info("✓ Successfully authenticated with Gmail")

        # Test connection
        results = service.users().messages().list(userId='me', maxResults=1).execute()
        logger.info("✓ Gmail API connection working")

        return 0
    except Exception as e:
        logger.error(f"Setup failed: {e}")
        return 1

def cmd_export(args):
    """Export data to various formats"""
    logger = Logger()
    config = Config()

    try:
        db = DatabaseManager(config.database, config.db_password)
        db.connect()

        output_path = Path(args.output) if args.output else None
        export_format = args.format.lower()

        if export_format == 'pdf':
            from gmail_tracker.exporter import PDFExporter
            exporter = PDFExporter()

            if args.type == 'summary':
                logger.info("Generating PDF summary report...")
                content = exporter.generate_summary_report(db)
            elif args.type == 'analytics':
                period = args.period or 'month'
                logger.info(f"Generating PDF analytics report for {period}...")
                content = exporter.generate_analytics_report(db, period=period)
            elif args.type == 'application' and args.app_id:
                logger.info(f"Generating PDF for application {args.app_id}...")
                content = exporter.generate_application_detail(db, args.app_id)
            else:
                logger.info("Generating PDF summary report...")
                content = exporter.generate_summary_report(db)

            # Determine output path
            if not output_path:
                from datetime import datetime
                timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
                output_path = Path(f"job_tracker_{args.type}_{timestamp}.pdf")

            output_path.write_bytes(content)
            logger.info(f"PDF exported to: {output_path}")

        elif export_format == 'csv':
            from gmail_tracker.exporter import CSVExporter
            exporter = CSVExporter()

            if args.type == 'companies':
                logger.info("Exporting companies to CSV...")
                content = exporter.export_companies(db)
            elif args.type == 'backup':
                logger.info("Exporting full backup to CSV...")
                content = exporter.export_full_backup(db)
            else:
                logger.info("Exporting applications to CSV...")
                filters = {}
                if args.status:
                    filters['status'] = args.status
                content = exporter.export_applications(db, filters if filters else None)

            # Determine output path
            if not output_path:
                from datetime import datetime
                timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
                output_path = Path(f"job_tracker_{args.type}_{timestamp}.csv")

            output_path.write_text(content)
            logger.info(f"CSV exported to: {output_path}")

        elif export_format == 'json':
            from gmail_tracker.exporter import JSONExporter
            exporter = JSONExporter()

            logger.info("Exporting data to JSON...")
            content = exporter.export_to_string(db)

            # Determine output path
            if not output_path:
                from datetime import datetime
                timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
                output_path = Path(f"job_tracker_backup_{timestamp}.json")

            output_path.write_text(content)
            logger.info(f"JSON exported to: {output_path}")

        else:
            logger.error(f"Unknown export format: {export_format}")
            return 1

        db.disconnect()
        return 0

    except Exception as e:
        logger.error(f"Export failed: {e}")
        return 1


def cmd_health(args):
    """Health check"""
    logger = Logger()
    logger.info("Running health check")

    # Check Gmail auth
    auth = GmailAuthenticator()
    if auth.is_authenticated():
        logger.info("✓ Gmail API: Authenticated")
    else:
        logger.error("✗ Gmail API: Not authenticated (run --setup)")
        return 1

    # Check database
    try:
        config = Config()
        from gmail_tracker.database import DatabaseManager
        db = DatabaseManager(config.database, config.db_password)
        db.connect()

        # Get counts
        db.cursor.execute("SELECT COUNT(*) FROM companies")
        company_count = db.cursor.fetchone()['count']

        db.cursor.execute("SELECT COUNT(*) FROM applications")
        app_count = db.cursor.fetchone()['count']

        logger.info(f"✓ Database: Connected ({company_count} companies, {app_count} applications)")
        db.disconnect()
    except Exception as e:
        logger.error(f"✗ Database: Connection failed - {e}")
        return 1

    # Check config
    logger.info("✓ Config file: Valid")

    logger.info("Health check passed")
    return 0

def main():
    """Main entry point"""
    parser = argparse.ArgumentParser(
        description=f"Gmail Job Application Tracker v{__version__}"
    )
    parser.add_argument('--version', action='version', version=f'%(prog)s {__version__}')

    subparsers = parser.add_subparsers(dest='command', help='Command to run')

    # Run command
    run_parser = subparsers.add_parser('run', help='Run the job tracker')
    run_parser.add_argument('--dry-run', action='store_true',
                           help='Show what would be done without inserting to database')
    run_parser.set_defaults(func=cmd_run)

    # Setup command
    setup_parser = subparsers.add_parser('setup', help='Run initial OAuth setup')
    setup_parser.add_argument('--remote', action='store_true',
                              help='Use console flow for SSH/remote authentication')
    setup_parser.set_defaults(func=cmd_setup)

    # Health command
    health_parser = subparsers.add_parser('health', help='Run health check')
    health_parser.set_defaults(func=cmd_health)

    # Export command
    export_parser = subparsers.add_parser('export', help='Export data to PDF, CSV, or JSON')
    export_parser.add_argument('--format', '-f', choices=['pdf', 'csv', 'json'],
                               default='pdf', help='Export format (default: pdf)')
    export_parser.add_argument('--output', '-o', help='Output file path')
    export_parser.add_argument('--type', '-t',
                               choices=['summary', 'analytics', 'applications', 'companies', 'backup', 'application'],
                               default='summary',
                               help='Type of export (default: summary)')
    export_parser.add_argument('--period', '-p', choices=['week', 'month', 'quarter', 'year'],
                               default='month', help='Time period for analytics (default: month)')
    export_parser.add_argument('--status', '-s', help='Filter by status (for CSV applications export)')
    export_parser.add_argument('--app-id', type=int, help='Application ID (for single application PDF)')
    export_parser.set_defaults(func=cmd_export)

    # Default to run if no command
    args = parser.parse_args()
    if not args.command:
        args.command = 'run'
        args.dry_run = False
        args.func = cmd_run

    return args.func(args)

if __name__ == "__main__":
    sys.exit(main())
