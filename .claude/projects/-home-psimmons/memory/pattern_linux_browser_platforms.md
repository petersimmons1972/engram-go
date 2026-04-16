---
name: Linux Browser Platform Compatibility
description: Enterprise browser platforms (HireVue etc.) that claim Win/Mac only usually work on Linux with user-agent spoofing
type: feedback
Category: reference
originSessionId: f3bd39be-e957-4489-afa7-cb9309dc7a88
---
Enterprise browser-based platforms that list only Windows/Mac support usually work fine on Linux Chrome/Firefox. If the platform blocks Linux:

1. Spoof user-agent to Windows Chrome via DevTools → Network conditions → uncheck "Use browser default"
2. Or launch: `google-chrome --user-agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"`
3. Test webcam/mic first: `chrome://settings/content/camera`
4. Fallback: use the mobile app

**Why:** Peter runs Linux as primary desktop. Many hiring platforms don't officially support it but work anyway.
**How to apply:** When Peter hits a "not supported on your OS" wall with any browser-based platform, try user-agent spoofing before switching devices.
