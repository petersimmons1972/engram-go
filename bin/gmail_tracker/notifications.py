"""Notification system for Gmail Job Tracker

Provides email and Slack notifications for job application events.
"""

import smtplib
import logging
import json
from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart
from dataclasses import dataclass, field
from datetime import datetime
from typing import Dict, Any, List, Optional
from pathlib import Path
import urllib.request
import urllib.error

logger = logging.getLogger(__name__)


class NotificationError(Exception):
    """Raised when a notification fails to send"""
    pass


@dataclass
class NotificationPreferences:
    """User preferences for notifications"""
    daily_digest: bool = True
    new_application_alerts: bool = True
    status_change_alerts: bool = True
    interview_reminders: bool = True
    email_enabled: bool = True
    slack_enabled: bool = False
    slack_webhook_url: Optional[str] = None
    daily_digest_time: str = "09:00"

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> 'NotificationPreferences':
        """Create preferences from dictionary

        Args:
            data: Dictionary with preference values

        Returns:
            NotificationPreferences instance
        """
        return cls(
            daily_digest=data.get('daily_digest', True),
            new_application_alerts=data.get('new_application_alerts', True),
            status_change_alerts=data.get('status_change_alerts', True),
            interview_reminders=data.get('interview_reminders', True),
            email_enabled=data.get('email_enabled', True),
            slack_enabled=data.get('slack_enabled', False),
            slack_webhook_url=data.get('slack_webhook_url'),
            daily_digest_time=data.get('daily_digest_time', '09:00')
        )

    def to_dict(self) -> Dict[str, Any]:
        """Convert preferences to dictionary

        Returns:
            Dictionary representation
        """
        return {
            'daily_digest': self.daily_digest,
            'new_application_alerts': self.new_application_alerts,
            'status_change_alerts': self.status_change_alerts,
            'interview_reminders': self.interview_reminders,
            'email_enabled': self.email_enabled,
            'slack_enabled': self.slack_enabled,
            'slack_webhook_url': self.slack_webhook_url,
            'daily_digest_time': self.daily_digest_time
        }


@dataclass
class Notification:
    """Represents a notification to be sent"""
    id: str
    event_type: str
    data: Dict[str, Any]
    created_at: datetime
    sent: bool = False
    sent_at: Optional[datetime] = None
    channels: List[str] = field(default_factory=list)


class EmailNotifier:
    """Sends email notifications via SMTP"""

    def __init__(self, smtp_host: str = 'smtp.gmail.com',
                 smtp_port: int = 587,
                 smtp_user: str = None,
                 smtp_password: str = None,
                 use_tls: bool = True):
        """Initialize email notifier

        Args:
            smtp_host: SMTP server hostname
            smtp_port: SMTP server port
            smtp_user: SMTP username (email address)
            smtp_password: SMTP password or app password
            use_tls: Whether to use TLS encryption
        """
        self.smtp_host = smtp_host
        self.smtp_port = smtp_port
        self.smtp_user = smtp_user
        self.smtp_password = smtp_password
        self.use_tls = use_tls

    def _create_connection(self) -> smtplib.SMTP:
        """Create SMTP connection

        Returns:
            Connected SMTP instance

        Raises:
            NotificationError: If connection fails
        """
        try:
            server = smtplib.SMTP(self.smtp_host, self.smtp_port)
            if self.use_tls:
                server.starttls()
            if self.smtp_user and self.smtp_password:
                server.login(self.smtp_user, self.smtp_password)
            return server
        except smtplib.SMTPException as e:
            raise NotificationError(f"Failed to connect to SMTP server: {e}")
        except Exception as e:
            raise NotificationError(f"SMTP connection error: {e}")

    def _send_email(self, to_email: str, subject: str, html_body: str,
                    text_body: str = None) -> bool:
        """Send an email

        Args:
            to_email: Recipient email address
            subject: Email subject
            html_body: HTML email body
            text_body: Plain text body (optional, generated from HTML if not provided)

        Returns:
            True if sent successfully

        Raises:
            NotificationError: If sending fails
        """
        if not self.smtp_user:
            raise NotificationError("SMTP user not configured")

        msg = MIMEMultipart('alternative')
        msg['Subject'] = subject
        msg['From'] = self.smtp_user
        msg['To'] = to_email

        # Add plain text version
        if text_body is None:
            # Simple HTML to text conversion
            text_body = html_body.replace('<br>', '\n').replace('</p>', '\n')
            import re
            text_body = re.sub('<[^<]+?>', '', text_body)

        msg.attach(MIMEText(text_body, 'plain'))
        msg.attach(MIMEText(html_body, 'html'))

        try:
            server = self._create_connection()
            server.sendmail(self.smtp_user, to_email, msg.as_string())
            server.quit()
            logger.info(f"Email sent to {to_email}: {subject}")
            return True
        except NotificationError:
            raise
        except Exception as e:
            raise NotificationError(f"Failed to send email: {e}")

    def send_daily_digest(self, user_email: str, stats: Dict[str, Any]) -> bool:
        """Send daily summary of application activity

        Args:
            user_email: Recipient email address
            stats: Dictionary containing activity statistics:
                - total_applications: int
                - new_applications: int
                - status_changes: List[Dict]
                - upcoming_interviews: List[Dict]
                - date: str (YYYY-MM-DD)

        Returns:
            True if sent successfully

        Raises:
            NotificationError: If sending fails
        """
        date = stats.get('date', datetime.now().strftime('%Y-%m-%d'))
        subject = f"Job Application Digest - {date}"

        html_body = f"""
        <html>
        <body style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
            <h2 style="color: #333;">Daily Application Digest</h2>
            <p style="color: #666;">Summary for {date}</p>

            <div style="background: #f5f5f5; padding: 15px; border-radius: 5px; margin: 15px 0;">
                <h3 style="margin-top: 0;">Overview</h3>
                <ul style="list-style: none; padding: 0;">
                    <li><strong>Total Applications:</strong> {stats.get('total_applications', 0)}</li>
                    <li><strong>New Today:</strong> {stats.get('new_applications', 0)}</li>
                </ul>
            </div>
        """

        # Add status changes section
        status_changes = stats.get('status_changes', [])
        if status_changes:
            html_body += """
            <div style="margin: 15px 0;">
                <h3>Status Updates</h3>
                <ul>
            """
            for change in status_changes:
                html_body += f"""
                    <li>
                        <strong>{change.get('company', 'Unknown')}</strong> - {change.get('position', 'Unknown')}<br>
                        <span style="color: #666;">{change.get('old_status', '?')} &rarr; {change.get('new_status', '?')}</span>
                    </li>
                """
            html_body += "</ul></div>"

        # Add upcoming interviews section
        interviews = stats.get('upcoming_interviews', [])
        if interviews:
            html_body += """
            <div style="background: #e8f4e8; padding: 15px; border-radius: 5px; margin: 15px 0;">
                <h3 style="margin-top: 0; color: #2e7d32;">Upcoming Interviews</h3>
                <ul>
            """
            for interview in interviews:
                html_body += f"""
                    <li>
                        <strong>{interview.get('company', 'Unknown')}</strong> - {interview.get('position', 'Unknown')}<br>
                        <span style="color: #666;">{interview.get('date', 'TBD')} at {interview.get('time', 'TBD')}</span>
                    </li>
                """
            html_body += "</ul></div>"

        html_body += """
            <p style="color: #999; font-size: 12px; margin-top: 30px;">
                This is an automated message from Gmail Job Tracker.
            </p>
        </body>
        </html>
        """

        return self._send_email(user_email, subject, html_body)

    def send_new_application_alert(self, user_email: str,
                                   application: Dict[str, Any]) -> bool:
        """Alert when new application detected

        Args:
            user_email: Recipient email address
            application: Application details:
                - company: str
                - position: str
                - application_date: str
                - job_url: Optional[str]

        Returns:
            True if sent successfully

        Raises:
            NotificationError: If sending fails
        """
        company = application.get('company', 'Unknown Company')
        position = application.get('position', 'Unknown Position')
        subject = f"New Application Tracked: {company} - {position}"

        job_url = application.get('job_url')
        url_html = f'<p><a href="{job_url}">View Job Posting</a></p>' if job_url else ''

        html_body = f"""
        <html>
        <body style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
            <h2 style="color: #1976d2;">New Application Tracked</h2>

            <div style="background: #e3f2fd; padding: 15px; border-radius: 5px;">
                <h3 style="margin-top: 0;">{company}</h3>
                <p style="margin: 5px 0;"><strong>Position:</strong> {position}</p>
                <p style="margin: 5px 0;"><strong>Applied:</strong> {application.get('application_date', 'Today')}</p>
                {url_html}
            </div>

            <p style="color: #999; font-size: 12px; margin-top: 30px;">
                This is an automated message from Gmail Job Tracker.
            </p>
        </body>
        </html>
        """

        return self._send_email(user_email, subject, html_body)

    def send_status_change_alert(self, user_email: str,
                                 application: Dict[str, Any],
                                 old_status: str,
                                 new_status: str) -> bool:
        """Alert when application status changes

        Args:
            user_email: Recipient email address
            application: Application details
            old_status: Previous status
            new_status: New status

        Returns:
            True if sent successfully

        Raises:
            NotificationError: If sending fails
        """
        company = application.get('company', 'Unknown Company')
        position = application.get('position', 'Unknown Position')
        subject = f"Status Update: {company} - {new_status}"

        # Color code based on status
        status_colors = {
            'applied': '#1976d2',
            'screening': '#ff9800',
            'interviewing': '#4caf50',
            'offer': '#2e7d32',
            'rejected': '#d32f2f',
            'withdrawn': '#9e9e9e'
        }
        color = status_colors.get(new_status.lower(), '#333')

        html_body = f"""
        <html>
        <body style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
            <h2 style="color: {color};">Application Status Changed</h2>

            <div style="background: #f5f5f5; padding: 15px; border-radius: 5px;">
                <h3 style="margin-top: 0;">{company}</h3>
                <p style="margin: 5px 0;"><strong>Position:</strong> {position}</p>
                <p style="margin: 5px 0;">
                    <strong>Status:</strong>
                    <span style="text-decoration: line-through; color: #999;">{old_status}</span>
                    &rarr;
                    <span style="color: {color}; font-weight: bold;">{new_status}</span>
                </p>
            </div>

            <p style="color: #999; font-size: 12px; margin-top: 30px;">
                This is an automated message from Gmail Job Tracker.
            </p>
        </body>
        </html>
        """

        return self._send_email(user_email, subject, html_body)

    def send_interview_reminder(self, user_email: str,
                                interview: Dict[str, Any],
                                hours_before: int = 24) -> bool:
        """Remind about upcoming interview

        Args:
            user_email: Recipient email address
            interview: Interview details:
                - company: str
                - position: str
                - date: str
                - time: str
                - location: Optional[str]
                - interviewer: Optional[str]
                - notes: Optional[str]
            hours_before: Hours before interview this reminder is for

        Returns:
            True if sent successfully

        Raises:
            NotificationError: If sending fails
        """
        company = interview.get('company', 'Unknown Company')
        position = interview.get('position', 'Unknown Position')
        subject = f"Interview Reminder: {company} in {hours_before} hours"

        location = interview.get('location', 'Not specified')
        interviewer = interview.get('interviewer')
        notes = interview.get('notes')

        interviewer_html = f'<p style="margin: 5px 0;"><strong>Interviewer:</strong> {interviewer}</p>' if interviewer else ''
        notes_html = f'<p style="margin: 5px 0;"><strong>Notes:</strong> {notes}</p>' if notes else ''

        html_body = f"""
        <html>
        <body style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
            <h2 style="color: #4caf50;">Interview Reminder</h2>
            <p style="color: #666;">Your interview is in {hours_before} hours!</p>

            <div style="background: #e8f5e9; padding: 15px; border-radius: 5px; border-left: 4px solid #4caf50;">
                <h3 style="margin-top: 0;">{company}</h3>
                <p style="margin: 5px 0;"><strong>Position:</strong> {position}</p>
                <p style="margin: 5px 0;"><strong>Date:</strong> {interview.get('date', 'TBD')}</p>
                <p style="margin: 5px 0;"><strong>Time:</strong> {interview.get('time', 'TBD')}</p>
                <p style="margin: 5px 0;"><strong>Location:</strong> {location}</p>
                {interviewer_html}
                {notes_html}
            </div>

            <div style="margin-top: 20px;">
                <h4>Preparation Checklist:</h4>
                <ul>
                    <li>Review job description and requirements</li>
                    <li>Research the company</li>
                    <li>Prepare questions to ask</li>
                    <li>Test your video/audio (if virtual)</li>
                    <li>Plan your route (if in-person)</li>
                </ul>
            </div>

            <p style="color: #999; font-size: 12px; margin-top: 30px;">
                This is an automated message from Gmail Job Tracker.
            </p>
        </body>
        </html>
        """

        return self._send_email(user_email, subject, html_body)


class SlackNotifier:
    """Sends notifications to Slack via webhook"""

    def __init__(self, webhook_url: str):
        """Initialize Slack notifier

        Args:
            webhook_url: Slack incoming webhook URL
        """
        if not webhook_url:
            raise ValueError("Slack webhook URL is required")
        self.webhook_url = webhook_url

    def send_message(self, text: str, blocks: List[Dict] = None) -> bool:
        """Send Slack message via webhook

        Args:
            text: Fallback text message
            blocks: Optional Slack blocks for rich formatting

        Returns:
            True if sent successfully

        Raises:
            NotificationError: If sending fails
        """
        payload = {'text': text}
        if blocks:
            payload['blocks'] = blocks

        try:
            data = json.dumps(payload).encode('utf-8')
            req = urllib.request.Request(
                self.webhook_url,
                data=data,
                headers={'Content-Type': 'application/json'}
            )
            with urllib.request.urlopen(req, timeout=10) as response:
                if response.status == 200:
                    logger.info(f"Slack message sent successfully")
                    return True
                else:
                    raise NotificationError(f"Slack API returned status {response.status}")
        except urllib.error.URLError as e:
            raise NotificationError(f"Failed to send Slack message: {e}")
        except Exception as e:
            raise NotificationError(f"Slack notification error: {e}")

    def format_application_message(self, application: Dict[str, Any]) -> List[Dict]:
        """Format application as Slack blocks

        Args:
            application: Application details

        Returns:
            List of Slack block elements
        """
        company = application.get('company', 'Unknown Company')
        position = application.get('position', 'Unknown Position')
        status = application.get('status', 'applied')
        job_url = application.get('job_url')

        blocks = [
            {
                "type": "header",
                "text": {
                    "type": "plain_text",
                    "text": f"New Application: {company}",
                    "emoji": True
                }
            },
            {
                "type": "section",
                "fields": [
                    {
                        "type": "mrkdwn",
                        "text": f"*Position:*\n{position}"
                    },
                    {
                        "type": "mrkdwn",
                        "text": f"*Status:*\n{status.title()}"
                    }
                ]
            }
        ]

        if job_url:
            blocks.append({
                "type": "section",
                "text": {
                    "type": "mrkdwn",
                    "text": f"<{job_url}|View Job Posting>"
                }
            })

        blocks.append({
            "type": "context",
            "elements": [
                {
                    "type": "mrkdwn",
                    "text": f"Tracked by Gmail Job Tracker | {datetime.now().strftime('%Y-%m-%d %H:%M')}"
                }
            ]
        })

        return blocks

    def send_new_application(self, application: Dict[str, Any]) -> bool:
        """Send new application notification

        Args:
            application: Application details

        Returns:
            True if sent successfully
        """
        company = application.get('company', 'Unknown')
        position = application.get('position', 'Unknown')
        text = f"New application tracked: {company} - {position}"
        blocks = self.format_application_message(application)
        return self.send_message(text, blocks)

    def send_status_change(self, application: Dict[str, Any],
                          old_status: str, new_status: str) -> bool:
        """Send status change notification

        Args:
            application: Application details
            old_status: Previous status
            new_status: New status

        Returns:
            True if sent successfully
        """
        company = application.get('company', 'Unknown')
        position = application.get('position', 'Unknown')
        text = f"Status update: {company} - {position}: {old_status} -> {new_status}"

        # Status emoji mapping
        status_emoji = {
            'applied': ':memo:',
            'screening': ':eyes:',
            'interviewing': ':speech_balloon:',
            'offer': ':star:',
            'rejected': ':x:',
            'withdrawn': ':wave:'
        }

        emoji = status_emoji.get(new_status.lower(), ':bell:')

        blocks = [
            {
                "type": "header",
                "text": {
                    "type": "plain_text",
                    "text": f"{emoji} Status Update",
                    "emoji": True
                }
            },
            {
                "type": "section",
                "fields": [
                    {
                        "type": "mrkdwn",
                        "text": f"*Company:*\n{company}"
                    },
                    {
                        "type": "mrkdwn",
                        "text": f"*Position:*\n{position}"
                    }
                ]
            },
            {
                "type": "section",
                "text": {
                    "type": "mrkdwn",
                    "text": f"~{old_status}~ -> *{new_status}*"
                }
            },
            {
                "type": "context",
                "elements": [
                    {
                        "type": "mrkdwn",
                        "text": f"Updated {datetime.now().strftime('%Y-%m-%d %H:%M')}"
                    }
                ]
            }
        ]

        return self.send_message(text, blocks)

    def send_interview_reminder(self, interview: Dict[str, Any],
                               hours_before: int = 24) -> bool:
        """Send interview reminder

        Args:
            interview: Interview details
            hours_before: Hours until interview

        Returns:
            True if sent successfully
        """
        company = interview.get('company', 'Unknown')
        position = interview.get('position', 'Unknown')
        date = interview.get('date', 'TBD')
        time = interview.get('time', 'TBD')

        text = f"Interview reminder: {company} - {position} in {hours_before} hours"

        blocks = [
            {
                "type": "header",
                "text": {
                    "type": "plain_text",
                    "text": ":calendar: Interview Reminder",
                    "emoji": True
                }
            },
            {
                "type": "section",
                "text": {
                    "type": "mrkdwn",
                    "text": f"Your interview is in *{hours_before} hours*!"
                }
            },
            {
                "type": "section",
                "fields": [
                    {
                        "type": "mrkdwn",
                        "text": f"*Company:*\n{company}"
                    },
                    {
                        "type": "mrkdwn",
                        "text": f"*Position:*\n{position}"
                    },
                    {
                        "type": "mrkdwn",
                        "text": f"*Date:*\n{date}"
                    },
                    {
                        "type": "mrkdwn",
                        "text": f"*Time:*\n{time}"
                    }
                ]
            }
        ]

        location = interview.get('location')
        if location:
            blocks.append({
                "type": "section",
                "text": {
                    "type": "mrkdwn",
                    "text": f"*Location:* {location}"
                }
            })

        return self.send_message(text, blocks)


class NotificationManager:
    """Manages notification routing and delivery"""

    def __init__(self, config: Dict[str, Any] = None,
                 preferences: NotificationPreferences = None):
        """Initialize notification manager

        Args:
            config: Notification configuration dict with structure:
                notifications:
                    email:
                        enabled: bool
                        smtp_host: str
                        smtp_port: int
                        smtp_user: str
                        smtp_password: str
                    slack:
                        enabled: bool
                        webhook_url: str
                    preferences: dict
            preferences: Optional NotificationPreferences to override config
        """
        self.config = config or {}
        self._notifications: List[Notification] = []
        self._notification_counter = 0

        # Initialize preferences
        if preferences:
            self.preferences = preferences
        else:
            pref_config = self.config.get('notifications', {}).get('preferences', {})
            self.preferences = NotificationPreferences.from_dict(pref_config)

        # Initialize notifiers
        self.email_notifier = None
        self.slack_notifier = None
        self._init_notifiers()

    def _init_notifiers(self):
        """Initialize notification channels based on config"""
        notif_config = self.config.get('notifications', {})

        # Email notifier
        email_config = notif_config.get('email', {})
        if email_config.get('enabled', False) and self.preferences.email_enabled:
            try:
                self.email_notifier = EmailNotifier(
                    smtp_host=email_config.get('smtp_host', 'smtp.gmail.com'),
                    smtp_port=email_config.get('smtp_port', 587),
                    smtp_user=email_config.get('smtp_user'),
                    smtp_password=email_config.get('smtp_password'),
                    use_tls=email_config.get('use_tls', True)
                )
                logger.info("Email notifier initialized")
            except Exception as e:
                logger.warning(f"Failed to initialize email notifier: {e}")

        # Slack notifier
        slack_config = notif_config.get('slack', {})
        webhook_url = slack_config.get('webhook_url') or self.preferences.slack_webhook_url
        if slack_config.get('enabled', False) and self.preferences.slack_enabled and webhook_url:
            try:
                self.slack_notifier = SlackNotifier(webhook_url)
                logger.info("Slack notifier initialized")
            except Exception as e:
                logger.warning(f"Failed to initialize Slack notifier: {e}")

    def notify(self, event_type: str, data: Dict[str, Any],
               user_email: str = None) -> bool:
        """Route notification to appropriate channels

        Args:
            event_type: Type of event ('new_application', 'status_change',
                       'interview_reminder', 'daily_digest')
            data: Event-specific data
            user_email: Email address for email notifications

        Returns:
            True if at least one notification was sent successfully
        """
        # Check if this event type is enabled
        if event_type == 'new_application' and not self.preferences.new_application_alerts:
            logger.debug(f"Skipping {event_type}: disabled in preferences")
            return False
        if event_type == 'status_change' and not self.preferences.status_change_alerts:
            logger.debug(f"Skipping {event_type}: disabled in preferences")
            return False
        if event_type == 'interview_reminder' and not self.preferences.interview_reminders:
            logger.debug(f"Skipping {event_type}: disabled in preferences")
            return False
        if event_type == 'daily_digest' and not self.preferences.daily_digest:
            logger.debug(f"Skipping {event_type}: disabled in preferences")
            return False

        # Create notification record
        self._notification_counter += 1
        notification = Notification(
            id=f"notif_{self._notification_counter}_{datetime.now().strftime('%Y%m%d%H%M%S')}",
            event_type=event_type,
            data=data,
            created_at=datetime.now()
        )

        success = False

        # Send email notification
        if self.email_notifier and user_email:
            try:
                if event_type == 'new_application':
                    self.email_notifier.send_new_application_alert(user_email, data)
                    notification.channels.append('email')
                    success = True
                elif event_type == 'status_change':
                    self.email_notifier.send_status_change_alert(
                        user_email,
                        data.get('application', {}),
                        data.get('old_status', ''),
                        data.get('new_status', '')
                    )
                    notification.channels.append('email')
                    success = True
                elif event_type == 'interview_reminder':
                    self.email_notifier.send_interview_reminder(
                        user_email,
                        data.get('interview', data),
                        data.get('hours_before', 24)
                    )
                    notification.channels.append('email')
                    success = True
                elif event_type == 'daily_digest':
                    self.email_notifier.send_daily_digest(user_email, data)
                    notification.channels.append('email')
                    success = True
            except NotificationError as e:
                logger.error(f"Email notification failed: {e}")

        # Send Slack notification
        if self.slack_notifier:
            try:
                if event_type == 'new_application':
                    self.slack_notifier.send_new_application(data)
                    notification.channels.append('slack')
                    success = True
                elif event_type == 'status_change':
                    self.slack_notifier.send_status_change(
                        data.get('application', {}),
                        data.get('old_status', ''),
                        data.get('new_status', '')
                    )
                    notification.channels.append('slack')
                    success = True
                elif event_type == 'interview_reminder':
                    self.slack_notifier.send_interview_reminder(
                        data.get('interview', data),
                        data.get('hours_before', 24)
                    )
                    notification.channels.append('slack')
                    success = True
            except NotificationError as e:
                logger.error(f"Slack notification failed: {e}")

        if success:
            notification.sent = True
            notification.sent_at = datetime.now()

        self._notifications.append(notification)
        return success

    def get_pending_notifications(self) -> List[Notification]:
        """Get notifications that need to be sent

        Returns:
            List of unsent notifications
        """
        return [n for n in self._notifications if not n.sent]

    def get_sent_notifications(self) -> List[Notification]:
        """Get notifications that have been sent

        Returns:
            List of sent notifications
        """
        return [n for n in self._notifications if n.sent]

    def mark_sent(self, notification_id: str) -> bool:
        """Mark notification as sent

        Args:
            notification_id: Notification ID

        Returns:
            True if found and marked
        """
        for notification in self._notifications:
            if notification.id == notification_id:
                notification.sent = True
                notification.sent_at = datetime.now()
                return True
        return False

    def clear_sent(self):
        """Remove sent notifications from internal list"""
        self._notifications = [n for n in self._notifications if not n.sent]
