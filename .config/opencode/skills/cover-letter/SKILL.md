---
name: cover-letter
description: Use when applying for jobs and need tailored cover letter - generates 6 diverse ideas, critiques harshly, synthesizes best elements with rationale
---

# Cover Letter Generation

Generate personalized cover letters through creative brainstorming and synthesis.

## When to Use

Use when applying for jobs and need a tailored cover letter based on your resume and the job description.

## Process

**Inputs:**
- Job description: Required (URL or pasted text)
- Resume: Optional (URL, pasted text, or defaults to /home/psimmons/Documents/Resume/Resume - Peter Simmons.pdf)

**Steps:**
1. Load resume (default if not provided)
2. Fetch/parse job description from URL or use pasted text
3. Analyze resume and job posting for key skills, experiences, requirements
4. Generate 6 creative cover letter ideas using distinct frameworks
5. Harshly critique each idea with specific criticisms and retained strengths
6. Synthesize final cover letter by combining best elements from all ideas
7. Output the final cover letter and detailed rationale

## Idea Frameworks

1. **Narrative Arc**: Hero's journey showing personal growth
2. **Quantified Impact**: Metrics and tangible results
3. **Vision Synergy**: Alignment with company mission
4. **Problem-Solution**: Addressing industry pain points
5. **Collaborative Ecosystem**: Teamwork emphasis
6. **Future Innovation**: Forward-thinking contributions

## Critique Criteria

- Relevance to job (1-10 scale)
- Originality vs templates
- Impact of value proposition
- Professional tone
- Logical structure and length

## Output Format

- **Final Cover Letter**: 3-4 paragraphs, tailored and compelling
- **Rationale**: Explanation of why each element was chosen from the critiques

## Error Handling

- If URL fetch fails, prompt for pasted text
- If default resume file missing, request user input
- Validate that inputs contain sufficient content for analysis

## Dependencies

- Web fetching for job description URLs
- PDF/text reading for resume
- Text analysis for key extraction
