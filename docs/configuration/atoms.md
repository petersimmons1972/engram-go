# Atom supersession

Atom extraction is enabled with `ENGRAM_ATOM_EXTRACTION_ENABLED`. Supersession
is enabled by default when extraction is enabled: a sufficiently confident new
atom with a different value replaces an active atom having the same project,
atom type, normalized subject, and normalized predicate.

The replacement row links to its predecessor through `supersedes`. Persistence
inserts that linked replacement and then retires the predecessor in one database
transaction. The predecessor's `valid_to` is the replacement's `observed_at`
(the source session's assertion date), not the worker's wall-clock time.

Events are occurrences and never supersede one another. Status changes from one
extraction batch are ordered by `observed_at`, with extractor order breaking
ties, and form a chain whose final value is the only active status. A replayed
status older than the active status is ignored.

A candidate supersedes only when its confidence is no more than `0.2` below the
active atom's confidence. Lower-confidence contradictions are inserted as
coexisting unlinked atoms and logged.

## First-run audit mode

Set `ENGRAM_ATOM_SUPERSESSION_DRY_RUN=true` for initial production runs. The
worker logs each would-supersede pair with both IDs and values, inserts each
candidate as a plain unlinked atom, and performs no retirement. The setting
defaults to `false`; remove it or set it to `false` after reviewing the audit
logs to enable transactional supersession.
