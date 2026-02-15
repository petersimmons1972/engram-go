# Watchdog Agent

## Description

The Watchdog subagent for opencode is designed to monitor terminal sessions for locked or unresponsive states, preventing the system from hanging. It achieves this by implementing timeouts on operations, detecting unresponsive processes, and maintaining a journal to preserve session state for recovery.

## How It Works

- **Monitoring and Detection**: The agent continuously monitors the execution of tools and commands within opencode. It uses configurable timeouts to identify when a process becomes unresponsive (e.g., a command that hangs indefinitely).

- **Timeout Handling**: When a timeout is exceeded, the agent can automatically terminate the hanging process, alert the user, or take other predefined actions to restore responsiveness.

- **Journaling**: To preserve session state, the agent logs all actions, tool calls, and responses to a journal file. This allows users to review what happened and potentially resume interrupted sessions by replaying the journal.

- **Integration**: The agent integrates seamlessly with opencode's tool execution system, hooking into the pipeline without disrupting normal operations.

## Configuration

```yaml
name: watchdog
version: 1.0
enabled: true
settings:
  timeout_ms: 300000  # Default timeout for operations (5 minutes)
  journal_file: "~/.opencode/journal.log"  # Path to the session journal
  actions_on_timeout:
    - kill_process
    - log_event
    - notify_user
  max_journal_size_mb: 100  # Rotate journal when it exceeds this size
```

This configuration enables the agent with default settings. Adjust the timeout and journal path as needed for your environment.