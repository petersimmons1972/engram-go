# Gmail Job Application Tracker

Automated system for monitoring Gmail and extracting job application data.

## Quick Start

### 1. Install Dependencies

```bash
# Create virtual environment
python3 -m venv ~/.venv/gmail-tracker
source ~/.venv/gmail-tracker/bin/activate

# Install packages
pip install google-auth-oauthlib google-auth-httplib2 \
    google-api-python-client anthropic psycopg2-binary \
    python-Levenshtein pyyaml
```

### 2. Google Cloud Setup

1. Go to https://console.cloud.google.com
2. Create project: "Gmail Job Tracker"
3. Enable Gmail API
4. Create OAuth 2.0 credentials (Desktop app)
5. Download `client_secret.json` → `~/.config/gmail-job-tracker/`

### 3. Configuration

Create `~/.config/gmail-job-tracker/config.yaml` (see design doc for template)

Create secret files:
```bash
echo "your-anthropic-key" > ~/.config/gmail-job-tracker/anthropic_api_key
echo "your-db-password" > ~/.config/gmail-job-tracker/db_password
chmod 600 ~/.config/gmail-job-tracker/*_api_key
chmod 600 ~/.config/gmail-job-tracker/db_password
```

### 4. Initial Authentication

```bash
~/bin/gmail-job-tracker.py setup
```

This opens browser for OAuth consent.

### 5. Test Run

```bash
# Dry run (don't insert to database)
~/bin/gmail-job-tracker.py run --dry-run

# Real run
~/bin/gmail-job-tracker.py run
```

### 6. Install Cron Job

```bash
crontab -e

# Add line (8am and 8pm daily):
0 8,20 * * * /home/psimmons/bin/gmail-job-tracker-wrapper.sh >> /home/psimmons/logs/gmail-job-tracker-cron.log 2>&1
```

## Commands

- `gmail-job-tracker.py run` - Run tracker
- `gmail-job-tracker.py run --dry-run` - Test without database insert
- `gmail-job-tracker.py setup` - Initial OAuth setup
- `gmail-job-tracker.py health` - Health check
- `gmail-job-tracker.py --help` - Show help

## Dashboard

A Streamlit-based web dashboard for viewing and managing job applications.

### Install Dashboard Dependencies

```bash
pip install streamlit plotly pandas
```

### Run Dashboard

```bash
cd ~/bin/gmail_tracker
streamlit run dashboard.py
```

Or from anywhere:

```bash
streamlit run ~/bin/gmail_tracker/dashboard.py
```

The dashboard will open in your browser at `http://localhost:8501`

### Dashboard Features

- **Dashboard**: Overview with key metrics, recent applications, status breakdown
- **Applications**: Filter, search, and manage all applications; add new applications manually
- **Analytics**: Charts showing applications over time, status breakdown, top companies, response rate trends
- **Settings**: Gmail connection status, export data to CSV, configuration overview

## Logs

- Main log: `~/logs/gmail-job-tracker.log`
- Errors: `~/logs/gmail-job-tracker-errors.log`
- Failed emails: `~/logs/gmail-job-tracker-errors/`

## Testing

```bash
cd ~/bin
python -m pytest gmail_tracker/tests/ -v
```

## Architecture

See `/home/psimmons/projects/job-search-system/docs/2026-01-15-gmail-job-tracker-design.md`
