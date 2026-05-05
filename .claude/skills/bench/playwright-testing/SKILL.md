---
name: playwright-testing
description: Debug flaky Playwright tests with race conditions, timing-dependent failures, stale element references, and intermittent timeouts. Use when tests fail intermittently, have element detachment issues, or require robust locator strategies with fallbacks. Handles error messages: "element not found", "timeout", "stale element", "network error". Covers TDD workflow, anti-patterns (hardcoding, brittle selectors, unclear assertions), debugging tools (locator, waitForElement, expect, evaluate), and exploration scripts.
---

# Playwright Testing Knowledge Session

**SHARED INFRASTRUCTURE:** Browser testing available to all generals. K8s service at `reports.petersimmons.com` serves test targets.

**Part of 12-layer QA ecosystem** - See QA-TOOLS.md for complete testing infrastructure.

## When To Use This Skill

- **Flaky tests**: Tests pass sometimes, fail randomly (race conditions, timing dependencies)
- **Intermittent failures**: "element not found" errors that don't consistently reproduce
- **Stale element errors**: Elements become detached or invalid during test execution
- **Timeout issues**: Timeouts waiting for elements, navigation, network responses
- **Brittleness**: Selectors break when HTML changes slightly (missing fallbacks)
- **Unclear failures**: Test assertions with unhelpful error messages
- **Element detachment**: "target page, context or browser has been closed" errors
- **Flakiness diagnosis**: Identifying whether failures are timing-dependent or structural

## Workflow

**Before writing any Playwright tests, ALWAYS:**

1. **Load Knowledge Base** - Read these files in order:
   - `/home/psimmons/projects/playwright-testing-knowledge/ANTI-PATTERNS.md` - Avoid known mistakes FIRST
   - `/home/psimmons/projects/playwright-testing-knowledge/BEST-PRACTICES.md` - Use proven patterns
   - `/home/psimmons/projects/playwright-testing-knowledge/LESSONS-LEARNED.md` - Learn from past experiences

2. **Before Writing Tests:**
   - Check ANTI-PATTERNS.md for mistakes to avoid
   - Use BEST-PRACTICES.md patterns (constants, fallback selectors, descriptive names)
   - If similar test exists in examples/, use as template

3. **When Debugging:**
   - Reference DEBUGGING-GUIDE.md: `/home/psimmons/projects/playwright-testing-knowledge/DEBUGGING-GUIDE.md`
   - Use `.evaluate()` for deep inspection (not just `.get_attribute()`)
   - Create exploration scripts to discover actual HTML structure
   - Never assume HTML structure - verify first

4. **After Learning Something:**
   - Document in LESSONS-LEARNED.md using the template
   - Extract reusable patterns to BEST-PRACTICES.md
   - Add mistakes to ANTI-PATTERNS.md
   - Commit to GitHub: `cd ~/projects/playwright-testing-knowledge && git add -A && git commit -m "lesson: <description>"`

## TDD Integration

**MANDATORY:** Follow Test-Driven Development workflow (use `superpowers:test-driven-development` skill):
1. **Write failing test first** - No production code without a failing test
2. **Watch it fail** - Verify the test fails for the right reason
3. **Write minimal code** - Just enough to pass
4. **Watch it pass** - Verify all tests green
5. **Refactor** - Clean up while keeping tests green

**Playwright-Specific TDD Cycle:**

**RED → EXPLORE → GREEN → REFACTOR**
- Write failing test based on expected behavior (RED)
- Explore actual HTML structure using debugging tools (EXPLORE)
- Fix test to match reality (GREEN)
- Extract constants, improve messages, add fallbacks (REFACTOR)

**Never Assume:**
- HTML structure (explore first!)
- Semantic elements exist
- Common ID/class patterns

**Always:**
- Use fallback selectors: `.sidebar.toc, .toc, nav`
- Extract configuration constants (REPORT_URL, SELECTORS)
- Provide context in assertion messages
- Collect all errors before failing

**Debug with:**
- `.evaluate()` for full DOM access
- Exploration scripts before writing tests
- `page.pause()` for manual inspection

## Anti-Patterns Table (Quick Reference)

| Anti-Pattern | Symptom | Correct Approach |
|---|---|---|
| **Assuming HTML structure** | "element not found" errors in other projects | Explore first with debugging script, use fallback selectors: `.sidebar, .toc, nav` |
| **Only using `.get_attribute()`** | Missing computed styles, parent relationships, visibility state | Use `.evaluate("el => ...")` for deep DOM inspection |
| **Hardcoded URLs/selectors** | Changes require editing multiple test functions | Extract to constants: `REPORT_URL`, `TOC_SELECTOR` at file top |
| **Vague assertion messages** | Failure doesn't explain what failed or why | Add context: `assert x, f"Expected {expected}, got {actual} (selector: {sel})"` |
| **Failing on first error** | Only see one problem, have to fix and re-run repeatedly | Collect errors in list, assert all at once before failing |
| **Skipping refactor phase** | Tests pass (Green) but remain brittle, unmaintainable | After Green, extract constants, improve messages, add fallbacks |
| **Not handling stale elements** | "target page, context or browser has been closed" errors | Use `.locator()` which re-queries each time, not stored references |
| **Missing timeout configuration** | Intermittent "timeout" errors on slow networks | Set explicit timeouts: `wait_for(timeout=10000)`, adjust per environment |
| **Race condition in assertions** | Intermittent failures on assertions | Use `expect()` API: `expect(element).to_be_visible()` (auto-retries) |
| **Not verifying element visibility** | Click fails silently, tests intermittently fail | Always check: `element.is_visible()` before interaction, handle hidden state |

## 12-Layer QA Ecosystem

Playwright is **Layer 5** of comprehensive testing infrastructure:

**Quick Reference:** See `~/projects/playwright-testing-knowledge/QA-TOOLS.md`

**Layers 1-7** (Already Implemented):
1. syrupy - Structural snapshots
2. pytest-regressions - Metric baselines
3. textstat - Readability
4. Citation URL validation
5. **Playwright** - Visual regression
6. Vale - Prose linting
7. G-Eval - Rubric scoring

**Layers 8-12** (Added 2026-02-13):
8. html5lib - HTML validation
9. axe-core - Accessibility
10. Internal link validation
11. Performance budgets
12. PyMuPDF - PDF quality

**Run All Layers:** `cd ~/projects/security-intelligence-business && ./bin/run-qa-tests.sh`

## Repository

All knowledge stored at: `~/projects/playwright-testing-knowledge/`

Git repository - commit lessons learned after each session.
