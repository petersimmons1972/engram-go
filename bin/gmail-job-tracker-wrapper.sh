#!/bin/bash
# Wrapper script for cron execution of gmail-job-tracker

set -e

# Load environment variables
export ANTHROPIC_API_KEY=$(cat ~/.config/gmail-job-tracker/anthropic_api_key 2>/dev/null || echo "")
export GMAIL_TRACKER_DB_PASSWORD=$(cat ~/.config/gmail-job-tracker/db_password 2>/dev/null || echo "")

# Activate virtual environment if it exists
if [ -d "$HOME/.venv/gmail-tracker" ]; then
    source "$HOME/.venv/gmail-tracker/bin/activate"
fi

# Change to script directory
cd "$HOME/bin"

# Run the tracker
python3 gmail-job-tracker.py run

# Deactivate venv if activated
if [ -n "$VIRTUAL_ENV" ]; then
    deactivate
fi
