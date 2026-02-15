# Cover Letter Skill Design - 2026-01-15

## Overview

The Cover Letter skill automates generating personalized cover letters through a structured creative process: brainstorming 6 diverse approaches, performing harsh critiques, synthesizing the best elements into a final letter, and providing rationale for choices.

## Purpose and Inputs

The Cover Letter skill will automate the process of generating personalized cover letters by following a structured creative workflow: brainstorm 6 diverse approaches, perform harsh critiques, synthesize the strongest elements into a final letter, and explain the rationale.

**Key inputs:**
- Resume content (pasted text, URL, or default PDF)
- Job description (pasted text or URL to posting)

**Success criteria:**
- Produces a compelling, tailored cover letter
- Avoids generic templates; emphasizes unique value proposition
- Length: 3-4 paragraphs suitable for email/submission

## Idea Generation Module

The skill analyzes the provided resume and job description to extract key elements: skills, experiences, achievements, and job requirements. It then generates 6 creative cover letter approaches, each with a distinct style and angle to maximize appeal.

**Creative Frameworks Applied:**
1. **Narrative Arc**: Structure as a hero's journey, showing personal growth and pivotal moments leading to this role
2. **Quantified Impact**: Focus on metrics, percentages, and tangible results from past roles
3. **Vision Synergy**: Align personal aspirations with the company's mission and industry disruption goals
4. **Problem-Solution Framework**: Identify industry pain points and position candidate as the solution
5. **Collaborative Ecosystem**: Emphasize cross-functional partnerships and team dynamics
6. **Future Innovation**: Highlight forward-thinking ideas and potential contributions to evolving technologies

Each idea includes 3-4 paragraph outlines with specific resume elements and job requirements woven in.

## Critique and Synthesis Process

After generating the 6 ideas, the skill performs harsh, constructive critiques using predefined criteria to identify strengths and weaknesses.

**Critique Framework:**
- **Relevance Score**: Alignment with job requirements (1-10 scale)
- **Originality Check**: Differentiation from standard templates
- **Impact Assessment**: Clarity of value proposition and achievements
- **Tone Evaluation**: Professional engagement without fluff
- **Structure Review**: Logical flow and appropriate length

For each idea, the skill outputs:
- 2-3 specific criticisms (e.g., "Too generic - doesn't mention specific Abnormal AI technologies")
- 1 key strength to preserve (e.g., "Strong narrative hook in opening")

**Synthesis Phase:**
- Extract top elements from all critiques
- Merge complementary strengths while eliminating identified weaknesses
- Produce final cover letter with reasoning document

**Output Format:**
- Final synthesized cover letter (3-4 paragraphs)
- Rationale document explaining why each element was chosen

## Implementation and Usage

The skill will be implemented as a structured workflow that can be invoked via the skill system.

**Skill File Location:** `~/.config/opencode/skills/cover-letter.md`

**Invocation:** Use `use_skill` with name "cover-letter"

**Parameters:**
- Job Description: Required (text paste or URL)
- Resume: Optional (text paste or URL); defaults to `/home/psimmons/Documents/Resume/Resume - Peter Simmons.pdf` if not provided

**Workflow Steps:**
1. If resume not provided, read from default PDF path
2. Validate and fetch job description (URL or text)
3. Extract key data from resume and job posting
4. Generate 6 creative cover letter ideas
5. Apply critiques to each idea
6. Synthesize final cover letter
7. Output letter + rationale

**Error Handling:** If URLs fail to fetch, prompt user to provide text directly. If default resume file missing, request user input.