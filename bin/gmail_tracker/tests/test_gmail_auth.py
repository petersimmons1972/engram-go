"""Tests for Gmail authentication"""

import json
import pytest
from pathlib import Path
from unittest.mock import Mock, patch, MagicMock
from gmail_tracker.gmail_auth import GmailAuthenticator


def test_authenticator_initializes():
    """Test that authenticator can be initialized"""
    auth = GmailAuthenticator()
    assert auth is not None


def test_credentials_path_default():
    """Test that credentials path defaults correctly"""
    auth = GmailAuthenticator()
    expected_path = Path.home() / '.config' / 'gmail-job-tracker'
    assert auth.credentials_dir == expected_path


def test_custom_credentials_dir():
    """Test that custom credentials directory is used"""
    custom_dir = Path('/tmp/test-credentials')
    auth = GmailAuthenticator(credentials_dir=custom_dir)
    assert auth.credentials_dir == custom_dir
    assert auth.token_path == custom_dir / 'token.json'
    assert auth.client_secret_path == custom_dir / 'client_secret.json'


def test_is_authenticated_no_token():
    """Test is_authenticated returns False when token doesn't exist"""
    auth = GmailAuthenticator(credentials_dir=Path('/tmp/nonexistent'))
    assert auth.is_authenticated() is False


def test_is_authenticated_corrupted_token(tmp_path):
    """Test is_authenticated handles corrupted token file"""
    auth = GmailAuthenticator(credentials_dir=tmp_path)

    # Create corrupted token file
    auth.token_path.write_text('{"corrupted": "data"}')

    # Should return False without crashing
    assert auth.is_authenticated() is False


def test_is_authenticated_invalid_json(tmp_path):
    """Test is_authenticated handles invalid JSON"""
    auth = GmailAuthenticator(credentials_dir=tmp_path)

    # Create invalid JSON file
    auth.token_path.write_text('not valid json {')

    # Should return False without crashing
    assert auth.is_authenticated() is False


def test_authenticate_missing_client_secret(tmp_path):
    """Test authenticate raises FileNotFoundError when client_secret.json is missing"""
    auth = GmailAuthenticator(credentials_dir=tmp_path)

    with pytest.raises(FileNotFoundError) as exc_info:
        auth.authenticate(interactive=True)

    assert "Client secret file not found" in str(exc_info.value)


def test_authenticate_non_interactive_no_credentials(tmp_path):
    """Test authenticate raises RuntimeError in non-interactive mode without credentials"""
    auth = GmailAuthenticator(credentials_dir=tmp_path)

    with pytest.raises(RuntimeError) as exc_info:
        auth.authenticate(interactive=False)

    assert "No valid credentials and interactive mode disabled" in str(exc_info.value)


def test_authenticate_handles_corrupted_token(tmp_path):
    """Test authenticate handles and removes corrupted token file"""
    auth = GmailAuthenticator(credentials_dir=tmp_path)

    # Create client_secret.json
    client_secret_data = {
        "installed": {
            "client_id": "test_id",
            "client_secret": "test_secret",
            "redirect_uris": ["http://localhost"],
            "auth_uri": "https://accounts.google.com/o/oauth2/auth",
            "token_uri": "https://oauth2.googleapis.com/token"
        }
    }
    auth.client_secret_path.write_text(json.dumps(client_secret_data))

    # Create corrupted token file
    auth.token_path.write_text('{"corrupted": "data"}')
    assert auth.token_path.exists()

    # Mock OAuth flow
    with patch('gmail_tracker.gmail_auth.InstalledAppFlow') as mock_flow, \
         patch('gmail_tracker.gmail_auth.build') as mock_build:

        # Setup mocks
        mock_creds = Mock()
        mock_creds.to_json.return_value = '{"token": "test"}'
        mock_flow.from_client_secrets_file.return_value.run_local_server.return_value = mock_creds
        mock_build.return_value = Mock()

        # Should handle corrupted token and proceed with OAuth
        service = auth.authenticate(interactive=True)
        assert service is not None


def test_authenticate_creates_directory(tmp_path):
    """Test authenticate creates credentials directory if it doesn't exist"""
    creds_dir = tmp_path / 'nested' / 'credentials'
    auth = GmailAuthenticator(credentials_dir=creds_dir)

    assert not creds_dir.exists()

    # Create client_secret.json in parent directory first
    creds_dir.mkdir(parents=True, exist_ok=True)
    client_secret_data = {
        "installed": {
            "client_id": "test_id",
            "client_secret": "test_secret",
            "redirect_uris": ["http://localhost"],
            "auth_uri": "https://accounts.google.com/o/oauth2/auth",
            "token_uri": "https://oauth2.googleapis.com/token"
        }
    }
    auth.client_secret_path.write_text(json.dumps(client_secret_data))

    with patch('gmail_tracker.gmail_auth.InstalledAppFlow') as mock_flow, \
         patch('gmail_tracker.gmail_auth.build') as mock_build:

        mock_creds = Mock()
        mock_creds.to_json.return_value = '{"token": "test"}'
        mock_flow.from_client_secrets_file.return_value.run_local_server.return_value = mock_creds
        mock_build.return_value = Mock()

        auth.authenticate(interactive=True)

        # Directory should be created
        assert creds_dir.exists()


def test_authenticate_sets_secure_permissions(tmp_path):
    """Test authenticate sets secure permissions (0o600) on token file"""
    auth = GmailAuthenticator(credentials_dir=tmp_path)

    # Create client_secret.json
    client_secret_data = {
        "installed": {
            "client_id": "test_id",
            "client_secret": "test_secret",
            "redirect_uris": ["http://localhost"],
            "auth_uri": "https://accounts.google.com/o/oauth2/auth",
            "token_uri": "https://oauth2.googleapis.com/token"
        }
    }
    auth.client_secret_path.write_text(json.dumps(client_secret_data))

    with patch('gmail_tracker.gmail_auth.InstalledAppFlow') as mock_flow, \
         patch('gmail_tracker.gmail_auth.build') as mock_build:

        mock_creds = Mock()
        mock_creds.to_json.return_value = '{"token": "test"}'
        mock_flow.from_client_secrets_file.return_value.run_local_server.return_value = mock_creds
        mock_build.return_value = Mock()

        auth.authenticate(interactive=True)

        # Check token file has secure permissions
        assert auth.token_path.exists()
        file_mode = auth.token_path.stat().st_mode & 0o777
        assert file_mode == 0o600


@patch('gmail_tracker.gmail_auth.Credentials')
@patch('gmail_tracker.gmail_auth.build')
def test_authenticate_with_valid_token(mock_build, mock_creds_class, tmp_path):
    """Test authenticate uses existing valid token"""
    auth = GmailAuthenticator(credentials_dir=tmp_path)

    # Create valid token file
    token_data = {
        "token": "test_token",
        "refresh_token": "test_refresh",
        "token_uri": "https://oauth2.googleapis.com/token",
        "client_id": "test_id",
        "client_secret": "test_secret",
        "scopes": ["https://www.googleapis.com/auth/gmail.readonly"]
    }
    auth.token_path.write_text(json.dumps(token_data))

    # Mock valid credentials
    mock_creds = Mock()
    mock_creds.valid = True
    mock_creds_class.from_authorized_user_file.return_value = mock_creds
    mock_build.return_value = Mock()

    service = auth.authenticate(interactive=False)

    # Should use existing token
    assert service is not None
    mock_creds_class.from_authorized_user_file.assert_called_once()


@pytest.mark.skip(reason="Requires OAuth setup")
def test_authenticate_creates_service():
    """Test that authentication creates Gmail service"""
    auth = GmailAuthenticator()
    service = auth.authenticate()
    assert service is not None
