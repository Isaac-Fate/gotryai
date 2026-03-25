package blogcomposer

import (
	"fmt"
	"strings"
)

const promptAnalyzeDraft = `You read rough notes for a technical blog post. Extract three fields:
- post_type: one of "tutorial", "exploration", "deep-dive", "opinion", "hybrid"
- audience: one sentence describing who benefits
- core_claim: one sentence — what the post argues, demonstrates, or teaches

Rough notes:
{{.draft}}

Return JSON only, matching this schema:
{{.schema}}`

const promptResearchQueries = `Plan 4-6 web search queries for a technical blog post. Queries should
give a wide overview of the topic: official docs, GitHub repos, announcements, comparisons, guides.
Each query should be distinct — do not rephrase the same thing.

Topic / rough notes:
{{.draft}}

Detected post type: {{.post_type}}
Core claim: {{.core_claim}}

Return JSON only, matching this schema:
{{.schema}}`

const promptResearchSynthesize = `You are a research assistant for a technical blog. From WEB SEARCH RESULTS,
produce a structured knowledge base.

Rules:
- overview: 2-3 paragraphs summarizing the landscape (tools, concepts, current state).
- evidence_items: 8-15 items. Each has a "fact" (2-3 sentences: the takeaway + what the source covers)
  and an optional "url" copied verbatim from the results. Do NOT invent URLs.
- key_resources: 6-12 notable links with title, url, and optional note.

Topic context:
{{.draft}}

--- WEB SEARCH RESULTS ---
{{.search}}
--- END ---

Return JSON only, matching this schema:
{{.schema}}`

const promptDesignBlueprint = `You are a creative editorial architect for technical blog posts.

From the draft notes, detected analysis, and knowledge base, design a Blueprint — the full
creative structure for the post.

HARD RULES:
- 5-8 sections. Each section has a type: "narrative", "how-to", "comparison", "deep-dive", "aside", "conclusion".
- VARY section types. A good post mixes them: narrative → how-to → aside → deep-dive → comparison → conclusion.
  NEVER produce more than 2 consecutive sections of the same type.
- Plan artifacts explicitly. Each section lists what it needs: "mermaid-architecture", "mermaid-flow",
  "mermaid-sequence", "code-go", "code-bash", "code-yaml", "code-json", "table", or none.
  MINIMUM across the whole post: at least 2 mermaid diagrams and at least 2 code blocks.
- Creative headings. NOT "Step 1: Install X" or "Introduction". Use headings that reveal the section's
  angle: "Getting the pieces on the board", "Where things go sideways", "What nobody tells you about X".
- narrative_arc: 1-2 sentences describing the post's emotional/intellectual journey.
  Example: "Start with the frustration of configuring by hand, build toward the clarity of the graph model,
  end with honest gaps."
- The FIRST section should be type "narrative" — it's the hook, not a TL;DR or bullet list.
- The LAST section should be type "conclusion" — honest, opinionated, forward-looking.

Draft notes:
{{.draft}}

Post type: {{.post_type}}
Audience: {{.audience}}
Core claim: {{.core_claim}}

Knowledge base (from web research):
{{.knowledge_base}}

{{.must_include_block}}

Return JSON only, matching this schema:
{{.schema}}`

const promptVoicePass = `You are injecting human voice into a technical blog post.

The post below is factually complete and well-structured, but reads like it was generated.
Your job: make it sound like a real engineer wrote it on their personal blog.

Rules:
- Add 2-4 personal asides or parenthetical opinions. Examples:
  "I'll be honest, this took me longer to figure out than I'd like to admit."
  "If you're anything like me, your first instinct is to skip this step. Don't."
  "(Spoiler: the answer was in the logs the whole time.)"
- Add human transitions between sections where the flow feels robotic.
- Voice to channel: {{.voice}}
- DO NOT change any code blocks, mermaid diagrams, URLs, or factual claims.
- DO NOT add filler phrases ("In today's world", "It is worth noting", etc.).
- DO NOT change headings.
- Keep the same overall length — you're adding texture, not padding.

--- BEGIN POST ---
{{.post}}
--- END POST ---

Output only the improved markdown.`

const promptFinalEdit = `You are a senior technical editor. Polish this blog post for publication.

Rules:
- Fix heading hierarchy (single # title, ## sections, ### subsections).
- Remove accidental duplicates, contradictions, or repeated phrases across sections.
- Ensure every mermaid block uses triple-backtick "mermaid" fencing.
- Ensure every code block specifies a language.
- Preserve ALL markdown links [text](url) exactly — same URLs. Do not drop or fabricate links.
- Remove any remaining filler phrases: "In today's world", "It is worth noting", "In conclusion",
  "indispensable", "game-changer", "at the end of the day".
- If two section openings use the same rhetorical pattern, rewrite one.
- Do NOT pad the post. If it's thin, leave it — don't add fluff.

Output only the final markdown.

--- BEGIN POST ---
{{.post}}
--- END POST ---`

func buildSectionPrompt(spec SectionSpec, blueprint Blueprint, kb string,
	prior string, voice string, draft string) string {

	typeBlock := sectionTypeInstructions(spec.Type)
	artifactBlock := artifactInstructions(spec.Artifacts)

	return fmt.Sprintf(`Write ONE section of a technical blog post in Markdown.

SECTION SPEC:
- Heading: ## %s
- Type: %s
- Purpose: %s
- Position in narrative arc: %s

%s

%s

CONTEXT:
- Article title: %s
- Thesis: %s
- Audience: %s
- Voice: %s

Knowledge base (from web research — use URLs from here, never invent):
%s

%s

Original rough draft (do not contradict):
%s

GLOBAL RULES:
- Mermaid diagrams MUST use triple-backtick "mermaid" fencing.
- Code blocks MUST specify language (go, bash, yaml, etc.).
- Links are inline [text](url) using only URLs from the knowledge base.
- No filler: ban "In today's world", "It is worth noting", "indispensable", "game-changer".
- Prefer "I found that..." over "It has been observed that..."
- Do NOT start with the heading text restated as a sentence.
- VARY your opening: sometimes a blunt claim, sometimes a question, sometimes a scenario.

Output ONLY the section markdown (## heading + body). No preamble or explanation.`,
		spec.Title, spec.Type, spec.Purpose, blueprint.NarrativeArc,
		typeBlock, artifactBlock,
		blueprint.Title, blueprint.Thesis, blueprint.Audience, voice,
		kb, prior, draft,
	)
}

func sectionTypeInstructions(stype string) string {
	switch stype {
	case "narrative":
		return `TYPE INSTRUCTIONS (narrative):
- Story-driven: set the scene, use an analogy or concrete scenario.
- No bullet lists in this section — flowing prose only.
- 400-700 words.
- This section should make the reader care about the problem before jumping to solutions.`
	case "how-to":
		return `TYPE INSTRUCTIONS (how-to):
- Walk through concrete steps with code blocks between brief explanations.
- Include at least one gotcha or "watch out for this" note.
- 500-800 words.
- Numbered steps or clear sequential flow, but weave explanation between them.`
	case "comparison":
		return `TYPE INSTRUCTIONS (comparison):
- Compare approaches, tools, or trade-offs.
- Use a markdown table OR side-by-side structure to make differences scannable.
- Then 1-2 paragraphs analyzing: when to pick each option.
- 400-600 words.`
	case "deep-dive":
		return `TYPE INSTRUCTIONS (deep-dive):
- Explain internals, architecture, or how something works under the hood.
- MUST include a mermaid diagram (flowchart, sequence, or class) illustrating the concept.
- Teach, don't just describe. Help the reader build a mental model.
- 500-900 words.`
	case "aside":
		return `TYPE INSTRUCTIONS (aside):
- Short personal note, opinion, or anecdote. Conversational tone.
- This is where personality lives: share what surprised you, what frustrated you, or what you'd do differently.
- 100-250 words. Keep it tight.`
	case "conclusion":
		return `TYPE INSTRUCTIONS (conclusion):
- Honest verdict: what's good, what's rough, what you'd explore next.
- DO NOT start with "In conclusion" or "To summarize."
- End with a concrete forward-looking thought or recommendation.
- 200-400 words.`
	default:
		return fmt.Sprintf(`TYPE INSTRUCTIONS (%s):
- Write a substantive section appropriate to the purpose described above.
- 400-700 words.`, stype)
	}
}

func artifactInstructions(artifacts []string) string {
	if len(artifacts) == 0 {
		return "ARTIFACTS: None required for this section."
	}
	var parts []string
	for _, a := range artifacts {
		switch {
		case a == "mermaid-architecture" || a == "mermaid-flow" || a == "mermaid-sequence":
			parts = append(parts, fmt.Sprintf("- Include a %s mermaid diagram (``` mermaid fenced).", a))
		case len(a) > 5 && a[:5] == "code-":
			lang := a[5:]
			parts = append(parts, fmt.Sprintf("- Include a %s code listing (``` %s fenced). Make it realistic and runnable if possible.", lang, lang))
		case a == "table":
			parts = append(parts, "- Include a markdown comparison table.")
		default:
			parts = append(parts, fmt.Sprintf("- Include: %s", a))
		}
	}
	return "REQUIRED ARTIFACTS for this section:\n" + joinLines(parts)
}

func joinLines(lines []string) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteString("\n")
	}
	return b.String()
}
