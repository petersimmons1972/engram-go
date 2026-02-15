"""Tests for notification system"""

import pytest
from unittest.mock import Mock, MagicMock, patch
from datetime import datetime
import json

from gmail_tracker.notifications import (
    NotificationError,
    NotificationPreferences,
    Notification,
    EmailNotifier,
    SlackNotifier,
    NotificationManager
)


class TestNotificationPreferences:
    """Tests for NotificationPreferences dataclass"""

    def test_default_preferences(self):
        """Test default preference values"""
        prefs = NotificationPreferences()
        assert prefs.daily_digest is True
        assert prefs.new_application_alerts is True
        assert prefs.status_change_alerts is True
        assert prefs.interview_reminders is True
        assert prefs.email_enabled is True
        assert prefs.slack_enabled is False
        assert prefs.slack_webhook_url is None
        assert prefs.daily_digest_time == "09:00"

    def test_from_dict_with_all_values(self):
        """Test creating preferences from dict with all values"""
        data = {
            'daily_digest': False,
            'new_application_alerts': False,
            'status_change_alerts': False,
            'interview_reminders': False,
            'email_enabled': False,
            'slack_enabled': True,
            'slack_webhook_url': 'https://hooks.slack.com/services/xxx',
            'daily_digest_time': '08:00'
        }
        prefs = NotificationPreferences.from_dict(data)

        assert prefs.daily_digest is False
        assert prefs.new_application_alerts is False
        assert prefs.status_change_alerts is False
        assert prefs.interview_reminders is False
        assert prefs.email_enabled is False
        assert prefs.slack_enabled is True
        assert prefs.slack_webhook_url == 'https://hooks.slack.com/services/xxx'
        assert prefs.daily_digest_time == '08:00'

    def test_from_dict_with_partial_values(self):
        """Test creating preferences from dict with partial values uses defaults"""
        data = {
            'daily_digest': False,
            'slack_enabled': True
        }
        prefs = NotificationPreferences.from_dict(data)

        # Specified values
        assert prefs.daily_digest is False
        assert prefs.slack_enabled is True

        # Default values
        assert prefs.new_application_alerts is True
        assert prefs.email_enabled is True
        assert prefs.daily_digest_time == '09:00'

    def test_from_dict_with_empty_dict(self):
        """Test creating preferences from empty dict uses all defaults"""
        prefs = NotificationPreferences.from_dict({})

        assert prefs.daily_digest is True
        assert prefs.new_application_alerts is True
        assert prefs.email_enabled is True
        assert prefs.slack_enabled is False

    def test_to_dict(self):
        """Test converting preferences to dict"""
        prefs = NotificationPreferences(
            daily_digest=False,
            slack_enabled=True,
            slack_webhook_url='https://example.com/webhook'
        )
        result = prefs.to_dict()

        assert isinstance(result, dict)
        assert result['daily_digest'] is False
        assert result['slack_enabled'] is True
        assert result['slack_webhook_url'] == 'https://example.com/webhook'
        assert result['email_enabled'] is True  # Default value

    def test_roundtrip_from_dict_to_dict(self):
        """Test that from_dict -> to_dict preserves values"""
        original = {
            'daily_digest': False,
            'new_application_alerts': True,
            'status_change_alerts': False,
            'interview_reminders': True,
            'email_enabled': True,
            'slack_enabled': True,
            'slack_webhook_url': 'https://test.webhook.url',
            'daily_digest_time': '10:30'
        }
        prefs = NotificationPreferences.from_dict(original)
        result = prefs.to_dict()

        assert result == original


class TestNotification:
    """Tests for Notification dataclass"""

    def test_notification_creation(self):
        """Test creating a notification"""
        notif = Notification(
            id='test_123',
            event_type='new_application',
            data={'company': 'Test Corp'},
            created_at=datetime.now()
        )

        assert notif.id == 'test_123'
        assert notif.event_type == 'new_application'
        assert notif.data['company'] == 'Test Corp'
        assert notif.sent is False
        assert notif.sent_at is None
        assert notif.channels == []

    def test_notification_with_channels(self):
        """Test notification with channels specified"""
        notif = Notification(
            id='test_456',
            event_type='status_change',
            data={},
            created_at=datetime.now(),
            channels=['email', 'slack']
        )

        assert 'email' in notif.channels
        assert 'slack' in notif.channels


class TestEmailNotifier:
    """Tests for EmailNotifier class"""

    def test_init_with_defaults(self):
        """Test EmailNotifier initialization with defaults"""
        notifier = EmailNotifier()

        assert notifier.smtp_host == 'smtp.gmail.com'
        assert notifier.smtp_port == 587
        assert notifier.use_tls is True
        assert notifier.smtp_user is None
        assert notifier.smtp_password is None

    def test_init_with_custom_values(self):
        """Test EmailNotifier initialization with custom values"""
        notifier = EmailNotifier(
            smtp_host='mail.example.com',
            smtp_port=465,
            smtp_user='user@example.com',
            smtp_password='secret',
            use_tls=False
        )

        assert notifier.smtp_host == 'mail.example.com'
        assert notifier.smtp_port == 465
        assert notifier.smtp_user == 'user@example.com'
        assert notifier.smtp_password == 'secret'
        assert notifier.use_tls is False

    def test_send_email_without_smtp_user_raises_error(self):
        """Test that sending email without smtp_user raises NotificationError"""
        notifier = EmailNotifier()

        with pytest.raises(NotificationError, match="SMTP user not configured"):
            notifier._send_email('test@example.com', 'Subject', '<p>Body</p>')

    @patch('gmail_tracker.notifications.smtplib.SMTP')
    def test_send_email_success(self, mock_smtp_class):
        """Test successful email sending"""
        mock_smtp = MagicMock()
        mock_smtp_class.return_value = mock_smtp

        notifier = EmailNotifier(
            smtp_user='sender@example.com',
            smtp_password='password'
        )

        result = notifier._send_email(
            'recipient@example.com',
            'Test Subject',
            '<p>Test Body</p>'
        )

        assert result is True
        mock_smtp.starttls.assert_called_once()
        mock_smtp.login.assert_called_once_with('sender@example.com', 'password')
        mock_smtp.sendmail.assert_called_once()
        mock_smtp.quit.assert_called_once()

    @patch('gmail_tracker.notifications.smtplib.SMTP')
    def test_send_email_smtp_failure(self, mock_smtp_class):
        """Test email sending failure raises NotificationError"""
        mock_smtp_class.side_effect = Exception("Connection failed")

        notifier = EmailNotifier(
            smtp_user='sender@example.com',
            smtp_password='password'
        )

        with pytest.raises(NotificationError, match="SMTP connection error"):
            notifier._send_email('recipient@example.com', 'Subject', '<p>Body</p>')

    @patch.object(EmailNotifier, '_send_email')
    def test_send_daily_digest(self, mock_send):
        """Test sending daily digest"""
        mock_send.return_value = True
        notifier = EmailNotifier(smtp_user='test@example.com')

        stats = {
            'date': '2026-01-17',
            'total_applications': 10,
            'new_applications': 2,
            'status_changes': [
                {'company': 'Test Corp', 'position': 'Engineer', 'old_status': 'applied', 'new_status': 'interviewing'}
            ],
            'upcoming_interviews': [
                {'company': 'Acme Inc', 'position': 'Developer', 'date': '2026-01-20', 'time': '10:00'}
            ]
        }

        result = notifier.send_daily_digest('user@example.com', stats)

        assert result is True
        mock_send.assert_called_once()
        args = mock_send.call_args
        assert args[0][0] == 'user@example.com'
        assert 'Job Application Digest' in args[0][1]
        assert '2026-01-17' in args[0][1]  # Date in subject
        assert '2026-01-17' in args[0][2]  # Date in body
        assert 'Test Corp' in args[0][2]
        assert 'Acme Inc' in args[0][2]

    @patch.object(EmailNotifier, '_send_email')
    def test_send_new_application_alert(self, mock_send):
        """Test sending new application alert"""
        mock_send.return_value = True
        notifier = EmailNotifier(smtp_user='test@example.com')

        application = {
            'company': 'Awesome Corp',
            'position': 'Senior Engineer',
            'application_date': '2026-01-17',
            'job_url': 'https://jobs.example.com/123'
        }

        result = notifier.send_new_application_alert('user@example.com', application)

        assert result is True
        args = mock_send.call_args
        assert 'Awesome Corp' in args[0][1]
        assert 'Senior Engineer' in args[0][1]
        assert 'https://jobs.example.com/123' in args[0][2]

    @patch.object(EmailNotifier, '_send_email')
    def test_send_status_change_alert(self, mock_send):
        """Test sending status change alert"""
        mock_send.return_value = True
        notifier = EmailNotifier(smtp_user='test@example.com')

        application = {
            'company': 'Tech Co',
            'position': 'Developer'
        }

        result = notifier.send_status_change_alert(
            'user@example.com',
            application,
            'applied',
            'interviewing'
        )

        assert result is True
        args = mock_send.call_args
        assert 'Tech Co' in args[0][1]
        assert 'interviewing' in args[0][1]
        assert 'applied' in args[0][2]
        assert 'interviewing' in args[0][2]

    @patch.object(EmailNotifier, '_send_email')
    def test_send_interview_reminder(self, mock_send):
        """Test sending interview reminder"""
        mock_send.return_value = True
        notifier = EmailNotifier(smtp_user='test@example.com')

        interview = {
            'company': 'Dream Corp',
            'position': 'Lead Engineer',
            'date': '2026-01-20',
            'time': '14:00',
            'location': 'Virtual - Zoom',
            'interviewer': 'Jane Smith',
            'notes': 'Bring portfolio'
        }

        result = notifier.send_interview_reminder('user@example.com', interview, hours_before=24)

        assert result is True
        args = mock_send.call_args
        assert 'Dream Corp' in args[0][1]
        assert '24 hours' in args[0][1]
        assert 'Jane Smith' in args[0][2]
        assert 'Virtual - Zoom' in args[0][2]


class TestSlackNotifier:
    """Tests for SlackNotifier class"""

    def test_init_requires_webhook_url(self):
        """Test that SlackNotifier requires a webhook URL"""
        with pytest.raises(ValueError, match="webhook URL is required"):
            SlackNotifier(webhook_url=None)

        with pytest.raises(ValueError, match="webhook URL is required"):
            SlackNotifier(webhook_url='')

    def test_init_with_webhook_url(self):
        """Test SlackNotifier initialization with webhook URL"""
        notifier = SlackNotifier('https://hooks.slack.com/services/xxx/yyy/zzz')
        assert notifier.webhook_url == 'https://hooks.slack.com/services/xxx/yyy/zzz'

    @patch('gmail_tracker.notifications.urllib.request.urlopen')
    def test_send_message_success(self, mock_urlopen):
        """Test successful Slack message sending"""
        mock_response = MagicMock()
        mock_response.status = 200
        mock_response.__enter__ = Mock(return_value=mock_response)
        mock_response.__exit__ = Mock(return_value=False)
        mock_urlopen.return_value = mock_response

        notifier = SlackNotifier('https://hooks.slack.com/test')
        result = notifier.send_message('Test message')

        assert result is True
        mock_urlopen.assert_called_once()

    @patch('gmail_tracker.notifications.urllib.request.urlopen')
    def test_send_message_with_blocks(self, mock_urlopen):
        """Test sending Slack message with blocks"""
        mock_response = MagicMock()
        mock_response.status = 200
        mock_response.__enter__ = Mock(return_value=mock_response)
        mock_response.__exit__ = Mock(return_value=False)
        mock_urlopen.return_value = mock_response

        notifier = SlackNotifier('https://hooks.slack.com/test')
        blocks = [{"type": "section", "text": {"type": "mrkdwn", "text": "Hello"}}]
        result = notifier.send_message('Test', blocks=blocks)

        assert result is True

    @patch('gmail_tracker.notifications.urllib.request.urlopen')
    def test_send_message_failure(self, mock_urlopen):
        """Test Slack message sending failure"""
        mock_urlopen.side_effect = Exception("Network error")

        notifier = SlackNotifier('https://hooks.slack.com/test')

        with pytest.raises(NotificationError, match="Slack notification error"):
            notifier.send_message('Test message')

    def test_format_application_message(self):
        """Test formatting application as Slack blocks"""
        notifier = SlackNotifier('https://hooks.slack.com/test')
        application = {
            'company': 'Test Corp',
            'position': 'Engineer',
            'status': 'applied',
            'job_url': 'https://jobs.test.com/123'
        }

        blocks = notifier.format_application_message(application)

        assert isinstance(blocks, list)
        assert len(blocks) >= 3

        # Check header block
        header = blocks[0]
        assert header['type'] == 'header'
        assert 'Test Corp' in header['text']['text']

        # Check for job URL block
        url_found = False
        for block in blocks:
            if block.get('type') == 'section':
                text = block.get('text', {}).get('text', '')
                if 'jobs.test.com' in text:
                    url_found = True
                    break
        assert url_found

    def test_format_application_message_without_url(self):
        """Test formatting application without job URL"""
        notifier = SlackNotifier('https://hooks.slack.com/test')
        application = {
            'company': 'Test Corp',
            'position': 'Engineer',
            'status': 'applied'
        }

        blocks = notifier.format_application_message(application)

        # Should not have URL block
        for block in blocks:
            if block.get('type') == 'section':
                text = block.get('text', {}).get('text', '')
                assert 'View Job Posting' not in text or block.get('fields')

    @patch.object(SlackNotifier, 'send_message')
    def test_send_new_application(self, mock_send):
        """Test sending new application notification"""
        mock_send.return_value = True
        notifier = SlackNotifier('https://hooks.slack.com/test')

        application = {
            'company': 'Acme Inc',
            'position': 'Developer'
        }

        result = notifier.send_new_application(application)

        assert result is True
        mock_send.assert_called_once()
        args = mock_send.call_args
        assert 'Acme Inc' in args[0][0]
        assert 'Developer' in args[0][0]

    @patch.object(SlackNotifier, 'send_message')
    def test_send_status_change(self, mock_send):
        """Test sending status change notification"""
        mock_send.return_value = True
        notifier = SlackNotifier('https://hooks.slack.com/test')

        application = {
            'company': 'Tech Co',
            'position': 'Engineer'
        }

        result = notifier.send_status_change(application, 'applied', 'offer')

        assert result is True
        args = mock_send.call_args
        assert 'offer' in args[0][0]

    @patch.object(SlackNotifier, 'send_message')
    def test_send_interview_reminder(self, mock_send):
        """Test sending interview reminder"""
        mock_send.return_value = True
        notifier = SlackNotifier('https://hooks.slack.com/test')

        interview = {
            'company': 'Dream Corp',
            'position': 'Lead',
            'date': '2026-01-20',
            'time': '10:00',
            'location': 'Office'
        }

        result = notifier.send_interview_reminder(interview, hours_before=12)

        assert result is True
        args = mock_send.call_args
        assert '12 hours' in args[0][0]


class TestNotificationManager:
    """Tests for NotificationManager class"""

    def test_init_with_no_config(self):
        """Test initialization with no config"""
        manager = NotificationManager()

        assert manager.email_notifier is None
        assert manager.slack_notifier is None
        assert isinstance(manager.preferences, NotificationPreferences)

    def test_init_with_preferences(self):
        """Test initialization with custom preferences"""
        prefs = NotificationPreferences(
            daily_digest=False,
            slack_enabled=True
        )
        manager = NotificationManager(preferences=prefs)

        assert manager.preferences.daily_digest is False
        assert manager.preferences.slack_enabled is True

    def test_init_with_email_config(self):
        """Test initialization with email config"""
        config = {
            'notifications': {
                'email': {
                    'enabled': True,
                    'smtp_host': 'smtp.test.com',
                    'smtp_port': 587,
                    'smtp_user': 'user@test.com',
                    'smtp_password': 'secret'
                }
            }
        }
        manager = NotificationManager(config=config)

        assert manager.email_notifier is not None
        assert manager.email_notifier.smtp_host == 'smtp.test.com'
        assert manager.email_notifier.smtp_user == 'user@test.com'

    def test_init_with_slack_config(self):
        """Test initialization with Slack config"""
        config = {
            'notifications': {
                'slack': {
                    'enabled': True,
                    'webhook_url': 'https://hooks.slack.com/test'
                },
                'preferences': {
                    'slack_enabled': True
                }
            }
        }
        manager = NotificationManager(config=config)

        assert manager.slack_notifier is not None
        assert manager.slack_notifier.webhook_url == 'https://hooks.slack.com/test'

    def test_notify_disabled_event_returns_false(self):
        """Test that disabled events return False"""
        prefs = NotificationPreferences(new_application_alerts=False)
        manager = NotificationManager(preferences=prefs)

        result = manager.notify('new_application', {'company': 'Test'})

        assert result is False

    def test_notify_without_notifiers_returns_false(self):
        """Test notify without configured notifiers"""
        manager = NotificationManager()

        result = manager.notify('new_application', {'company': 'Test'}, 'test@example.com')

        assert result is False

    @patch.object(EmailNotifier, 'send_new_application_alert')
    def test_notify_new_application_email(self, mock_send):
        """Test notify routes new application to email"""
        mock_send.return_value = True

        config = {
            'notifications': {
                'email': {
                    'enabled': True,
                    'smtp_user': 'sender@test.com',
                    'smtp_password': 'secret'
                }
            }
        }
        manager = NotificationManager(config=config)

        data = {'company': 'Test Corp', 'position': 'Engineer'}
        result = manager.notify('new_application', data, 'user@example.com')

        assert result is True
        mock_send.assert_called_once_with('user@example.com', data)

    @patch.object(SlackNotifier, 'send_new_application')
    def test_notify_new_application_slack(self, mock_send):
        """Test notify routes new application to Slack"""
        mock_send.return_value = True

        config = {
            'notifications': {
                'slack': {
                    'enabled': True,
                    'webhook_url': 'https://hooks.slack.com/test'
                },
                'preferences': {
                    'slack_enabled': True
                }
            }
        }
        manager = NotificationManager(config=config)

        data = {'company': 'Test Corp', 'position': 'Engineer'}
        result = manager.notify('new_application', data)

        assert result is True
        mock_send.assert_called_once_with(data)

    @patch.object(EmailNotifier, 'send_status_change_alert')
    def test_notify_status_change(self, mock_send):
        """Test notify routes status change"""
        mock_send.return_value = True

        config = {
            'notifications': {
                'email': {
                    'enabled': True,
                    'smtp_user': 'sender@test.com',
                    'smtp_password': 'secret'
                }
            }
        }
        manager = NotificationManager(config=config)

        data = {
            'application': {'company': 'Test', 'position': 'Dev'},
            'old_status': 'applied',
            'new_status': 'interviewing'
        }
        result = manager.notify('status_change', data, 'user@example.com')

        assert result is True
        mock_send.assert_called_once()

    @patch.object(EmailNotifier, 'send_interview_reminder')
    def test_notify_interview_reminder(self, mock_send):
        """Test notify routes interview reminder"""
        mock_send.return_value = True

        config = {
            'notifications': {
                'email': {
                    'enabled': True,
                    'smtp_user': 'sender@test.com',
                    'smtp_password': 'secret'
                }
            }
        }
        manager = NotificationManager(config=config)

        data = {
            'interview': {
                'company': 'Test',
                'position': 'Dev',
                'date': '2026-01-20',
                'time': '10:00'
            },
            'hours_before': 24
        }
        result = manager.notify('interview_reminder', data, 'user@example.com')

        assert result is True
        mock_send.assert_called_once()

    @patch.object(EmailNotifier, 'send_daily_digest')
    def test_notify_daily_digest(self, mock_send):
        """Test notify routes daily digest"""
        mock_send.return_value = True

        config = {
            'notifications': {
                'email': {
                    'enabled': True,
                    'smtp_user': 'sender@test.com',
                    'smtp_password': 'secret'
                }
            }
        }
        manager = NotificationManager(config=config)

        data = {
            'total_applications': 5,
            'new_applications': 1
        }
        result = manager.notify('daily_digest', data, 'user@example.com')

        assert result is True
        mock_send.assert_called_once_with('user@example.com', data)

    def test_get_pending_notifications(self):
        """Test getting pending notifications"""
        manager = NotificationManager()

        # Add some notifications
        manager._notifications = [
            Notification(
                id='1', event_type='new_application', data={},
                created_at=datetime.now(), sent=False
            ),
            Notification(
                id='2', event_type='new_application', data={},
                created_at=datetime.now(), sent=True
            ),
            Notification(
                id='3', event_type='status_change', data={},
                created_at=datetime.now(), sent=False
            )
        ]

        pending = manager.get_pending_notifications()

        assert len(pending) == 2
        assert all(not n.sent for n in pending)

    def test_get_sent_notifications(self):
        """Test getting sent notifications"""
        manager = NotificationManager()

        manager._notifications = [
            Notification(
                id='1', event_type='new_application', data={},
                created_at=datetime.now(), sent=False
            ),
            Notification(
                id='2', event_type='new_application', data={},
                created_at=datetime.now(), sent=True, sent_at=datetime.now()
            )
        ]

        sent = manager.get_sent_notifications()

        assert len(sent) == 1
        assert sent[0].id == '2'

    def test_mark_sent(self):
        """Test marking notification as sent"""
        manager = NotificationManager()

        notif = Notification(
            id='test_notif', event_type='new_application', data={},
            created_at=datetime.now(), sent=False
        )
        manager._notifications.append(notif)

        result = manager.mark_sent('test_notif')

        assert result is True
        assert notif.sent is True
        assert notif.sent_at is not None

    def test_mark_sent_not_found(self):
        """Test marking non-existent notification"""
        manager = NotificationManager()

        result = manager.mark_sent('nonexistent')

        assert result is False

    def test_clear_sent(self):
        """Test clearing sent notifications"""
        manager = NotificationManager()

        manager._notifications = [
            Notification(
                id='1', event_type='new_application', data={},
                created_at=datetime.now(), sent=False
            ),
            Notification(
                id='2', event_type='new_application', data={},
                created_at=datetime.now(), sent=True
            ),
            Notification(
                id='3', event_type='status_change', data={},
                created_at=datetime.now(), sent=True
            )
        ]

        manager.clear_sent()

        assert len(manager._notifications) == 1
        assert manager._notifications[0].id == '1'

    @patch.object(EmailNotifier, 'send_new_application_alert')
    def test_notification_records_channels(self, mock_send):
        """Test that notification records which channels were used"""
        mock_send.return_value = True

        config = {
            'notifications': {
                'email': {
                    'enabled': True,
                    'smtp_user': 'sender@test.com',
                    'smtp_password': 'secret'
                }
            }
        }
        manager = NotificationManager(config=config)

        manager.notify('new_application', {'company': 'Test'}, 'user@example.com')

        sent = manager.get_sent_notifications()
        assert len(sent) == 1
        assert 'email' in sent[0].channels

    @patch.object(EmailNotifier, 'send_new_application_alert')
    @patch.object(SlackNotifier, 'send_new_application')
    def test_notify_both_channels(self, mock_slack, mock_email):
        """Test notify sends to both email and Slack"""
        mock_email.return_value = True
        mock_slack.return_value = True

        config = {
            'notifications': {
                'email': {
                    'enabled': True,
                    'smtp_user': 'sender@test.com',
                    'smtp_password': 'secret'
                },
                'slack': {
                    'enabled': True,
                    'webhook_url': 'https://hooks.slack.com/test'
                },
                'preferences': {
                    'slack_enabled': True
                }
            }
        }
        manager = NotificationManager(config=config)

        result = manager.notify('new_application', {'company': 'Test'}, 'user@example.com')

        assert result is True
        mock_email.assert_called_once()
        mock_slack.assert_called_once()

        sent = manager.get_sent_notifications()
        assert 'email' in sent[0].channels
        assert 'slack' in sent[0].channels

    @patch.object(EmailNotifier, 'send_new_application_alert')
    def test_notify_handles_email_failure_gracefully(self, mock_send):
        """Test that email failure is handled gracefully"""
        mock_send.side_effect = NotificationError("SMTP error")

        config = {
            'notifications': {
                'email': {
                    'enabled': True,
                    'smtp_user': 'sender@test.com',
                    'smtp_password': 'secret'
                }
            }
        }
        manager = NotificationManager(config=config)

        result = manager.notify('new_application', {'company': 'Test'}, 'user@example.com')

        # Should return False since notification failed, but not raise exception
        assert result is False


class TestNotificationPreferencesIntegration:
    """Integration tests for preference-based notification filtering"""

    def test_preferences_disable_daily_digest(self):
        """Test that disabled daily_digest preference prevents notification"""
        prefs = NotificationPreferences(daily_digest=False)
        manager = NotificationManager(preferences=prefs)

        result = manager.notify('daily_digest', {'stats': {}})

        assert result is False
        assert len(manager._notifications) == 0

    def test_preferences_disable_status_alerts(self):
        """Test that disabled status_change_alerts prevents notification"""
        prefs = NotificationPreferences(status_change_alerts=False)
        manager = NotificationManager(preferences=prefs)

        result = manager.notify('status_change', {
            'application': {},
            'old_status': 'applied',
            'new_status': 'interviewing'
        })

        assert result is False

    def test_preferences_disable_interview_reminders(self):
        """Test that disabled interview_reminders prevents notification"""
        prefs = NotificationPreferences(interview_reminders=False)
        manager = NotificationManager(preferences=prefs)

        result = manager.notify('interview_reminder', {
            'interview': {},
            'hours_before': 24
        })

        assert result is False

    def test_email_disabled_in_preferences(self):
        """Test that email_enabled=False prevents email notifier initialization"""
        config = {
            'notifications': {
                'email': {
                    'enabled': True,
                    'smtp_user': 'user@test.com',
                    'smtp_password': 'secret'
                }
            }
        }
        prefs = NotificationPreferences(email_enabled=False)
        manager = NotificationManager(config=config, preferences=prefs)

        # Email notifier should not be initialized when email_enabled=False in preferences
        assert manager.email_notifier is None
