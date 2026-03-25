package blogcomposer

import (
	"fmt"
	"strings"
)

// mermaidSyntaxRules is inlined into section and edit prompts so model output renders in
// common Mermaid renderers (GitHub, mermaid.live). See https://mermaid.js.org/syntax/flowchart.html
// (Markdown strings, special characters) and community notes on line breaks vs. literal \n.
const mermaidSyntaxRules = `MERMAID SYNTAX (critical — invalid diagrams break the post):
- Put each diagram in a fenced code block with the mermaid language tag. One graph or sequence per diagram.
- Do NOT use the two-character sequence backslash followed by n inside node or edge labels as a line break.
  In default flowcharts (HTML labels), that is not a newline; use one of:
  (1) HTML line breaks: <br/> inside the label, e.g. A["First line<br/>Second line"] or F(Label <br/> more).
  (2) Markdown string nodes (Mermaid "Markdown Strings"): use double quotes and backtick-wrapped text
      per Mermaid docs; break lines with real newlines in the markdown source, not the literal backslash-n characters.
- If node text must include the English word "end" alone, capitalize it (e.g. "End") — lowercase "end" breaks flowcharts.
- Flowchart links: if text after --- starts with "o" or "x", add a space or capitalize so it is not parsed as a circle/cross edge.
- Quote node text when it contains awkward characters (parentheses, commas, quotes).
- Prefer flowchart TD/LR or sequenceDiagram; keep subgraph ... end balanced.
- Sequence diagrams: short participant names; avoid backslash-n in titles — use <br/> instead.`

// citationRules requires inline markdown links so research-backed posts surface Key Resources in prose,
// not only as a detached list in the knowledge base.
// codeGroundingRules: one standard for every fenced block — no language gets a loophole.
const codeGroundingRules = `CODE INTEGRITY (critical — identical standard for every language tag: go, bash, yaml, json, etc.):
- A fenced block is allowed only when every non-stdlib identifier in it is traceable to the Knowledge base
  (linked file, README, or docs URL). Otherwise: no fence — prose plus links to official examples.
- No carve-outs: no “just illustrative,” “pseudo,” or “sketch” code inside fences.
- Do not name, compare, or recommend repos or products unless each appears with a URL in the Knowledge base.
- A tiny pure-stdlib snippet is allowed only if it has zero third-party imports and does not pretend to be a full project;
  that is not permission to mix in fake package names or wrong APIs for the topic at hand.`

const citationRules = `INLINE CITATIONS (critical — readers need clickable sources):
- Use Markdown [anchor text](url) in the body whenever you state a fact, name a project, or point to docs or repos.
  Anchor text should describe the destination (e.g. "the LangGraphGo docs", "this release note"), not "click here".
- URLs MUST appear verbatim from the Knowledge base below (Evidence source links or Key Resources). Never invent URLs.
- Do NOT dump links only in a "References" block at the end — weave at least half of this section's links into sentences.
- Do NOT use bare URLs without markdown; use [text](url).
- Minimum for THIS section: at least 2 distinct markdown links, except type "aside" (at least 1) and "conclusion"
  (at least 2 links to further reading or docs). If the Knowledge base has few URLs, use every one you have naturally.`

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
  and a "url" whenever the search results provide one — copy it verbatim. Prefer facts that include a URL.
  Do NOT invent URLs.
- key_resources: 8-14 notable links. Every item MUST include "url" copied verbatim from the results and a short "title".
  These URLs are what the blog will cite inline — maximize coverage of official docs, repos, and authoritative posts.

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
  MINIMUM across the whole post: at least 2 mermaid diagrams. Prefer at least 1 KB-grounded code artifact total
  (any language); prefer "code-bash" for copy-pastable commands that appear verbatim in the Knowledge base.
  Do not plan multiple code-* sections unless each can cite sources — otherwise use prose + links.
  Planned mermaid diagrams must be drawable: no literal "\\n" for line breaks in labels (use <br/>
  or markdown-string newlines per MERMAID rules in the writing stage).
- Creative headings. NOT "Step 1: Install X" or "Introduction". Use headings that reveal the section's
  angle: "Getting the pieces on the board", "Where things go sideways", "What nobody tells you about X".
- narrative_arc: 1-2 sentences describing the post's emotional/intellectual journey.
  Example: "Start with the frustration of configuring by hand, build toward the clarity of the graph model,
  end with honest gaps."
- The FIRST section should be type "narrative" — it's the hook, not a TL;DR or bullet list.
- The LAST section should be type "conclusion" — honest, opinionated, forward-looking.

LINK CITATION PLAN (required — the final post must cite research URLs in prose):
- Each section MUST include "cite_urls": an array of 1-4 URLs copied EXACTLY from the Knowledge base
  (from "### Key Resources" or Evidence lines). Do not invent or edit URLs.
- Spread citations across sections: no section should have an empty "cite_urls" unless the Knowledge base truly has no URLs.
- Assign URLs that match each section's topic (docs for how-to, architecture posts for deep-dive, etc.).
- Aim for the union of cite_urls across all sections to include at least 6 distinct URLs when the Knowledge base has that many.

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
- DO NOT change any mermaid diagrams, URLs, or factual claims. If any fenced code block is hallucinated
  (imports, commands, or APIs not supportable from the post’s sources), replace it with prose plus links to
  official examples — do not preserve fabricated code.
- Preserve every inline markdown link [text](url). Do not remove links or reduce how many appear.
- DO NOT add filler phrases ("In today's world", "It is worth noting", etc.).
- DO NOT change headings.
- Keep the same overall length — you're adding texture, not padding.

--- BEGIN POST ---
{{.post}}
--- END POST ---

Output only the improved markdown.`

const promptFinalEdit = `You are a senior technical editor. Polish this blog post for publication.

Knowledge base (use ONLY these URLs if you add or fix citations — never invent links):
{{.knowledge_base}}

Rules:
- Fix heading hierarchy (single # title, ## sections, ### subsections).
- Remove accidental duplicates, contradictions, or repeated phrases across sections.
- Ensure every mermaid block uses triple-backtick "mermaid" fencing.
- Fix broken Mermaid: remove literal "\\n" from inside node/edge labels where a line break was
  intended — replace with <br/> or restructure using markdown string nodes with real newlines.
  Fix "end" in node text (capitalize). Match opening/closing subgraph and fence.
- Ensure every code block specifies a language.
- Remove or rewrite any fenced code that invents imports, commands, or APIs not supported by linked sources;
  prefer linking to official examples or source files from the Knowledge base over long fabricated listings.
- Preserve ALL markdown links [text](url) exactly — same URLs. Do not drop or fabricate links.
- Citation density: if the article cites fewer than 8 distinct URLs in the body, add natural [text](url) links
  where sentences support claims (docs, repos, posts), using ONLY URLs from the Knowledge base above.
  Prefer weaving links into prose over a standalone link list.
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

%s

Original rough draft (do not contradict):
%s

GLOBAL RULES:
%s
%s
%s
- Mermaid diagrams MUST use triple-backtick "mermaid" fencing.
- Code blocks MUST specify language (go, bash, yaml, etc.).
- No filler: ban "In today's world", "It is worth noting", "indispensable", "game-changer".
- Prefer "I found that..." over "It has been observed that..."
- Do NOT start with the heading text restated as a sentence.
- VARY your opening: sometimes a blunt claim, sometimes a question, sometimes a scenario.

Output ONLY the section markdown (## heading + body). No preamble or explanation.`,
		spec.Title, spec.Type, spec.Purpose, blueprint.NarrativeArc,
		typeBlock, artifactBlock,
		blueprint.Title, blueprint.Thesis, blueprint.Audience, voice,
		kb, buildCitationBlock(spec), prior, draft,
		mermaidSyntaxRules, citationRules, codeGroundingRules,
	)
}

func sectionTypeInstructions(stype string) string {
	switch stype {
	case "narrative":
		return `TYPE INSTRUCTIONS (narrative):
- Story-driven: set the scene, use an analogy or concrete scenario.
- No bullet lists in this section — flowing prose only.
- 400-700 words.
- This section should make the reader care about the problem before jumping to solutions.
- Cite 2+ sources inline when you mention tools, papers, or ecosystems (see INLINE CITATIONS).`
	case "how-to":
		return `TYPE INSTRUCTIONS (how-to):
- Walk through concrete steps; fenced code only when it satisfies CODE INTEGRITY (KB-grounded).
- Include at least one gotcha or "watch out for this" note.
- 500-800 words.
- Numbered steps or clear sequential flow, but weave explanation between them.
- Where upstream behavior matters, link to official install/docs or a specific file URL from the Knowledge base instead of inventing listings.`
	case "comparison":
		return `TYPE INSTRUCTIONS (comparison):
- Compare approaches, tools, or trade-offs.
- Use a markdown table OR side-by-side structure to make differences scannable.
- Then 1-2 paragraphs analyzing: when to pick each option.
- 400-600 words.
- When naming a product or project, add an inline link to its site or repo from the Knowledge base.`
	case "deep-dive":
		return `TYPE INSTRUCTIONS (deep-dive):
- Explain internals, architecture, or how something works under the hood.
- MUST include a mermaid diagram (flowchart, sequence, or class) illustrating the concept.
  Keep labels Mermaid-valid (see MERMAID SYNTAX: line breaks via <br/> or real newlines in markdown strings, not backslash-n).
- Teach, don't just describe. Help the reader build a mental model.
- 500-900 words.
- Cite design docs, specs, or source repos inline when you explain behavior.`
	case "aside":
		return `TYPE INSTRUCTIONS (aside):
- Short personal note, opinion, or anecdote. Conversational tone.
- This is where personality lives: share what surprised you, what frustrated you, or what you'd do differently.
- 100-250 words. Keep it tight.
- Include at least one inline link if the aside references a specific tool or article.`
	case "conclusion":
		return `TYPE INSTRUCTIONS (conclusion):
- Honest verdict: what's good, what's rough, what you'd explore next.
- DO NOT start with "In conclusion" or "To summarize."
- End with a concrete forward-looking thought or recommendation.
- 200-400 words.
- Point readers to 2+ concrete URLs (docs, repos, further reading) from the Knowledge base.`
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
			parts = append(parts, fmt.Sprintf(
				"- Include a %s mermaid diagram (fenced). It must parse: use <br/> or markdown-string "+
					"newlines for multi-line labels — never literal backslash-n inside labels.",
				a,
			))
		case len(a) > 5 && a[:5] == "code-":
			lang := a[5:]
			parts = append(parts, fmt.Sprintf(
				"- Include a ```%s block only if fully grounded in the Knowledge base (see CODE INTEGRITY). "+
					"If not grounded, use prose and markdown links instead of a fence.", lang))
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

func buildCitationBlock(spec SectionSpec) string {
	if len(spec.CiteURLs) == 0 {
		return "SECTION CITATION PLAN: No cite_urls in blueprint for this section. Still add the minimum " +
			"inline [text](url) links from the Knowledge base required by INLINE CITATIONS below."
	}
	var b strings.Builder
	b.WriteString("SECTION CITATION PLAN (weave these URLs into prose as [anchor](url), not a naked list):\n")
	for _, u := range spec.CiteURLs {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(u)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}
