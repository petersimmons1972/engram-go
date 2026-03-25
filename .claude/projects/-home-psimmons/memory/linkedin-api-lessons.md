---
Category: reference
name: LinkedIn API Hard Lessons
description: Critical LinkedIn REST API bugs and quirks learned through production incidents
type: reference
---

# LinkedIn API Hard Lessons

## Critical Bugs

**1. Parentheses silently truncate posts.** Reserved chars `| { } @ [ ] ( ) < > # \ * _ ~` must be escaped in `commentary` field. LinkedIn truncates at first unescaped reserved char with no error. Use `escape_commentary()` in `lib/publish.py`. Discovered when post #001 lost 40% of text at `(i)`.

**2. Use REST Posts API, NOT legacy UGC.** Wrong: `POST /v2/ugcPosts`. Correct: `POST /rest/posts` with headers `LinkedIn-Version: 202601` and `Content-Type: application/json`.

**3. Person URN uses OpenID `sub`, NOT profile ID.** URN = `urn:li:person:{sub}` from OAuth token response. Not the numeric browser profile ID.

**4. Shell expansion mangles `$` in post text.** Use `--text-file post.md`, never `--text "$(cat post.md)"`.

**5. Token expiry is 60 days.** Check: `python bin/linkedin-auth.py check`. Stored at `~/.config/linkedin/credentials.age`.

**6. IPv4 only.** `publish.py` forces IPv4 automatically. Use `curl --ipv4` for manual debugging.

## Character Limits

- API hard limit: 3,000 chars in `commentary`
- Target: <1,500 chars (better performance)
- Hook zone: first ~210 chars (before "See more")

## Image/Document Upload

Images: `initializeUpload` → presigned PUT URL → POST with URN. Documents (PDFs/carousels): same flow via `/rest/documents`. Never upload raw bytes to `/rest/posts`.

## Other

- Duplicate detection: LinkedIn rejects identical text within ~10 min of deletion.
- Reference impl: `~/projects/linkedin/lib/publish.py`
