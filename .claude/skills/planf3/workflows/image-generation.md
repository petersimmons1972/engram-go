# Image Generation (Grok /imagine)

Fill or update embedded images in a plan `.html` file using the fleet **grok-agent**
(`grok.petersimmons.com`) via the `/imagine` slash command. Images are **opt-in**:
only run this when the user asks for images or a plan clearly benefits. Never block
plan completion on image generation — if grok is unreachable, leave the `{{...IMAGE}}`
slots as commented placeholders and report it.

| Sub-workflow | When to call it |
| --- | --- |
| Create | The prompt asks to generate/fill the plan's images from empty `{{...IMAGE` slots |
| Update | The prompt asks to change/regenerate images that already exist in the plan |

Output: grok produces a raster JPEG. Use **atmospheric, impressionistic, or metaphorical** language for every prompt — this routes to `image_gen` (success). **Never use structured diagram language** (`"labeled boxes"`, `"flat-vector technical architecture diagram"`, `"bar chart with exact values"`, `"labeled arrows"`) — grok's imagine skill intercepts these as "needs code for accuracy" and routes to code generation, which cancels in headless mode. Match the plan's `:root` palette (professional, focused, minimal).

**Prompt strategy by slot type:**

| Slot type | ✅ Routes to `image_gen` | ❌ Triggers code path (avoid) |
|-----------|--------------------------|-------------------------------|
| Architecture / flow | "glowing nodes converging into a single pipeline, dark background, neon tech" | "architecture diagram with labeled boxes and arrows" |
| Data comparison | "a tall vibrant green column dwarfing a smaller red column, dramatic contrast" | "bar chart comparing 37.9% vs 23.3%" |
| Hero / concept | "server rack glowing blue in a dark room, cinematic dramatic lighting" | "technical infrastructure diagram" |
| Pipeline / steps | "five glowing orbs linked by a beam of light flowing left to right" | "flow diagram with step labels" |

Shared rules for every image prompt:
- convey the one or two core ideas of that section for a professional software engineer
- keep total words shown in the image under 10 (image models garble text — avoid requesting specific labels)
- save images to `IMAGES_OUTPUT_DIR` (create it if missing)

## Create

1. Find slots - Grep the plan for `{{...IMAGE` placeholders. Each comment names the intended subject.
2. Write prompts - For each slot, write a `/imagine` prompt using atmospheric/impressionistic language (see table above). Never request labeled diagrams or exact numbers. Follow the shared rules.
3. Generate - For each slot, invoke grok and capture the full JSON response:
   ```bash
   PB=$(base64 -w0 prompt.txt)   # prompt.txt begins with: /imagine <description>
   RESPONSE=$(ssh grok.petersimmons.com "echo '$PB' | base64 -d | docker exec -i grok-agent grok --prompt-file /dev/stdin --output-format json --no-memory --permission-mode bypassPermissions --max-turns 20")
   echo "$RESPONSE"
   # Verify stopReason is "EndTurn" (not "Cancelled") before proceeding
   echo "$RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); exit(0 if d.get('stopReason')=='EndTurn' else 1)" || echo "WARNING: image generation did not complete"
   ```
4. Retrieve - The JSON response does NOT contain the image data. Parse `sessionId` from the response to find the exact path — do not use time-based `find`:
   ```bash
   SESSION_ID=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['sessionId'])")
   SESSION_PATH="/home/grok/.grok/sessions/%2Fhome%2Fgrok/${SESSION_ID}/images"
   # List available images in the session (1.jpg, 2.jpg, ... for multiple generations)
   ssh grok.petersimmons.com "docker exec grok-agent sh -c 'ls ${SESSION_PATH}/'"
   # Extract the first image (binary-safe: ssh + docker exec pipe works for JPEG)
   ssh grok.petersimmons.com "docker exec grok-agent sh -c 'cat ${SESSION_PATH}/1.jpg'" > IMAGES_OUTPUT_DIR/<name>.jpg
   ```
5. Embed - Replace each `<!-- {{...IMAGE: ...}} -->` placeholder with `<img src="<plan-name>/<name>.jpg" alt="...">`, keeping the existing `<figure>`/`<figcaption>`.
6. Report - List the images generated and the slots filled.

## Update

1. Identify targets - From the `USER_PROMPT`, determine which embedded images to change.
2. Regenerate - grok `/imagine` has no in-place edit; generate a fresh image with a new prompt describing the desired result, retrieve it (steps 3-4 above), and overwrite the target file (back up the original first with `cp <file> <file>.bak`).
3. Verify embed - Confirm the `<img>` still points at the file; update `src`/`alt`/`<figcaption>` if warranted.
4. Report - List the images updated and what changed.
