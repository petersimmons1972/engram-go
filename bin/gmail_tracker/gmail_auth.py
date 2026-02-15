"""Gmail API authentication management"""

import logging
import os
from pathlib import Path
from typing import Any
from google.auth.transport.requests import Request
from google.oauth2.credentials import Credentials
from google_auth_oauthlib.flow import InstalledAppFlow
from googleapiclient.discovery import build

# Set up logging
logger = logging.getLogger(__name__)

# Gmail API scope (read-only)
SCOPES = ['https://www.googleapis.com/auth/gmail.readonly']

class GmailAuthenticator:
    """Manages Gmail OAuth2 authentication"""

    def __init__(self, credentials_dir: Path = None):
        """Initialize authenticator

        Args:
            credentials_dir: Directory containing OAuth credentials
        """
        if credentials_dir is None:
            credentials_dir = Path.home() / '.config' / 'gmail-job-tracker'

        self.credentials_dir = credentials_dir
        self.token_path = credentials_dir / 'token.json'
        self.client_secret_path = credentials_dir / 'client_secret.json'

    def authenticate(self, interactive: bool = True) -> Any:
        """Authenticate and return Gmail service

        Args:
            interactive: If True, open browser for OAuth flow

        Returns:
            Gmail API service object

        Raises:
            FileNotFoundError: If client_secret.json is missing
            RuntimeError: If no valid credentials and interactive mode disabled
            ValueError: If token file is corrupted
        """
        creds = None

        # Load existing token if available
        if self.token_path.exists():
            try:
                creds = Credentials.from_authorized_user_file(str(self.token_path), SCOPES)
                logger.info("Loaded existing credentials from token file")
            except (ValueError, KeyError) as e:
                logger.warning(f"Corrupted token file detected: {e}. Will re-authenticate.")
                # Remove corrupted token file
                self.token_path.unlink()
                creds = None

        # Refresh or get new credentials
        if not creds or not creds.valid:
            if creds and creds.expired and creds.refresh_token:
                try:
                    logger.info("Refreshing expired credentials")
                    creds.refresh(Request())
                    logger.info("Credentials refreshed successfully")
                except Exception as e:
                    logger.error(f"Failed to refresh credentials: {e}")
                    raise
            elif interactive:
                # Validate client_secret.json exists
                if not self.client_secret_path.exists():
                    logger.error(f"Client secret file not found: {self.client_secret_path}")
                    raise FileNotFoundError(
                        f"Client secret file not found: {self.client_secret_path}. "
                        "Please download it from Google Cloud Console."
                    )

                logger.info("Starting OAuth flow")
                flow = InstalledAppFlow.from_client_secrets_file(
                    str(self.client_secret_path), SCOPES)
                try:
                    # Try local server first
                    creds = flow.run_local_server(port=0)
                    logger.info("OAuth flow completed successfully")
                except Exception as e:
                    # Fall back to console flow (manual code entry)
                    logger.warning(f"Local server failed ({e}), using console flow")
                    creds = self._run_console_flow(flow)
                    logger.info("OAuth flow completed via console method")
            else:
                logger.error("No valid credentials and interactive mode disabled")
                raise RuntimeError("No valid credentials and interactive mode disabled")

            # Save credentials for next run
            # Ensure directory exists
            self.credentials_dir.mkdir(parents=True, exist_ok=True)

            # Write token
            self.token_path.write_text(creds.to_json())

            # Set secure permissions (user read/write only)
            self.token_path.chmod(0o600)
            logger.info(f"Saved credentials to {self.token_path} with secure permissions")

        # Build and return service
        logger.info("Building Gmail API service")
        service = build('gmail', 'v1', credentials=creds)
        return service

    def _run_console_flow(self, flow: InstalledAppFlow) -> Credentials:
        """Run OAuth flow via console (for remote/SSH use)

        Args:
            flow: The OAuth flow object

        Returns:
            Credentials object
        """
        # Use loopback redirect - browser will fail but URL will contain code
        flow.redirect_uri = "http://localhost:1"
        auth_url, _ = flow.authorization_url(
            prompt='consent',
            access_type='offline'
        )
        print("\n" + "="*70)
        print("REMOTE AUTHENTICATION")
        print("="*70)
        print("\n1. Open this URL in any browser:\n")
        print(auth_url)
        print("\n2. Sign in and authorize the application")
        print("3. The browser will show an error (can't connect) - THIS IS EXPECTED")
        print("4. Look at the URL bar - it will show something like:")
        print("   http://localhost:1/?code=4/0AeanS0a...&scope=...")
        print("5. Copy the code value (everything between 'code=' and '&')")
        print("="*70)
        auth_code = input("\nPaste the authorization code here: ").strip()
        flow.fetch_token(code=auth_code)
        return flow.credentials

    def authenticate_console(self) -> Any:
        """Authenticate using console flow only (for SSH/remote use)

        Returns:
            Gmail API service object
        """
        if not self.client_secret_path.exists():
            raise FileNotFoundError(
                f"Client secret file not found: {self.client_secret_path}. "
                "Please download it from Google Cloud Console."
            )

        logger.info("Starting console OAuth flow")
        flow = InstalledAppFlow.from_client_secrets_file(
            str(self.client_secret_path), SCOPES)

        creds = self._run_console_flow(flow)

        # Save credentials
        self.credentials_dir.mkdir(parents=True, exist_ok=True)
        self.token_path.write_text(creds.to_json())
        self.token_path.chmod(0o600)
        logger.info(f"Saved credentials to {self.token_path}")

        # Build and return service
        service = build('gmail', 'v1', credentials=creds)
        return service

    def is_authenticated(self) -> bool:
        """Check if valid credentials exist

        Returns:
            True if authenticated, False otherwise
        """
        if not self.token_path.exists():
            logger.debug("Token file does not exist")
            return False

        try:
            creds = Credentials.from_authorized_user_file(str(self.token_path), SCOPES)
            is_valid = creds and creds.valid
            logger.debug(f"Credentials valid: {is_valid}")
            return is_valid
        except (ValueError, KeyError, OSError) as e:
            logger.warning(f"Error checking credentials: {e}")
            return False
