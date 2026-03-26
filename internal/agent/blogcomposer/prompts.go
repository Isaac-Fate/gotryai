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

// verbatimExcerptEthics states the trust contract for quoted listings; technical details are in verbatimCopyRules.
const verbatimExcerptEthics = `EXCERPTS ARE EVIDENCE (non-negotiable — readers will open **Code source:** and compare):
- A non-mermaid fenced block together with **Code source:** claims "this text appears on that page." The inner bytes MUST match
  a contiguous span in DuckDuckGo_Search / Fetch_Page_Text output or the KB / VERBATIM for this run. Changing characters inside the fence
  (rename symbols, reformat, "fix" typos, reorder lines, drop lines without elision markers) breaks that claim and misleads the reader — do not do it.
- If you want a tweaked, hypothetical, or pedagogical variant, put it ONLY in plain prose or a clearly labeled invented example
  (no **Code source:** pointing at a URL unless the bytes truly match that URL).`

const verbatimCopyRules = `COPYING CODE (non-mermaid fences):
- Inner text must match DuckDuckGo_Search output from this conversation or the Knowledge base / VERBATIM excerpts — same bytes as
  a contiguous span in the source. No renaming, re-ordering, “pretty-printing,” comment edits, or typo fixes.
- You MAY add markdown fences when the source left code unfenced: use a standard fenced block (opening line = three backticks + language
  tag, closing line = three backticks only). The code between opening and closing must be copied exactly from the source (only
  CRLF→LF line endings allowed). Do not touch the code itself.
- When the source already uses a fenced block, copy the inner code exactly (same rule). Opening/closing backticks must wrap that
  inner text only — no extra blank lines inside the fence beyond what the source had.
- Optional elision only: // ... elided ... (Go) or # ... elided ... (shell/yaml) on lines you remove; every other line must match source.
- If no matching listing exists, use prose + links only — no improvised code.`

const codeGroundingRules = `CODE INTEGRITY (go, bash, yaml, json, toml, etc.):
- Every application fence body must be an honest, character-accurate copy of a contiguous span from DuckDuckGo_Search output this run,
  Fetch_Page_Text, Raw web search in the KB, or VERBATIM. Wrapping unfenced source in a new fenced block (triple-backtick open/close)
  is allowed; changing characters inside is not — readers treat fences as quotations.
- mermaid: you may author diagrams (not API listings).
- Do not recommend tools without a URL from search results or the Knowledge base excerpts you were given.`

const codeSourceLineRules = `MANDATORY CODE PROVENANCE (readers must verify):
- For EVERY non-mermaid fenced block, the line IMMEDIATELY AFTER the closing triple-backtick must be exactly:
  **Code source:** [short human title](url)
  where url is copied verbatim from a DuckDuckGo_Search hit this session or from the Knowledge base / cite plan / Key Resources.
- Title should name the page (e.g. "langgraphgo examples README"). Never use "link" or "here" as anchor.
- If you cannot name a real URL for that snippet, delete the fence and use prose + link instead.
- mermaid blocks do NOT need **Code source:**.`

// substantiveBlogRules reflects common guidance for strong technical posts: prove claims, show working detail,
// teach mental models, and state limits — not marketing-shaped surveys. See e.g. industry technical-writing guides.
const substantiveBlogRules = `SUBSTANCE DENSITY (quality bar — fluff fails the reader):
- Each section must leave the reader with NEW, CHECKABLE knowledge: a named API/flag/type/command, a version constraint,
  a reproducible step, a specific failure mode or error string, or a tradeoff with WHEN to choose each option — backed by a link when it is a claim about behavior.
- Every paragraph must earn its space: it teaches a procedure, names a constraint, gives an example, or points to evidence.
  Cut throat-clearing: no "X is important" without the next sentence naming a concrete consequence; no defining obvious terms.
- Prefer specifics from sources over adjectives (avoid "powerful", "seamless", "robust" unless you immediately show what that means with a fact or listing).
- Honesty: if search/KB only has shallow pages, say what is unknown and link what exists — do not fill gaps with generic best-practice platitudes.
- Do not pad to hit word count: shorter + denser beats long + hollow.`

// authorColorRules: personality layered only on prose — never as edits inside quoted code.
const authorColorRules = `VOICE & INSIGHT (锦上添花 — prose and mermaid only, never inside non-mermaid code fences):
- Verbatim excerpts stay sacred (see EXCERPTS ARE EVIDENCE). All wit, metaphors, grumbles, and hot takes live in ordinary paragraphs,
  blockquotes of your words, or parentheticals — not between triple-backticks unless the language tag is mermaid.
- Add YOUR readalongs: "I'd watch for…", "what surprised me was…", "if I were shipping this tomorrow…" — clearly yours, not attributed to the docs.
- Light humor and vivid analogies are welcome when they clarify; avoid empty meme tone, punching down, or snark at the reader.
- Do not imply a source claimed something unless that claim is supported by linked evidence; opinions are explicit and separate.`

const citationRules = `INLINE CITATIONS (critical — readers need clickable sources):
- Use Markdown [anchor text](url) in the body whenever you state a fact, name a project, or point to docs or repos.
  Anchor text should describe the destination (e.g. "the LangGraphGo docs", "this release note"), not "click here".
- URLs MUST be copied verbatim from DuckDuckGo_Search results this session or the Knowledge base (Evidence, Key Resources, Raw web search).
  Never invent URLs.
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

const promptResearchQueries = `Plan 5-7 web search queries — still a skim, but surface material we can cite and optionally fetch as source.

Required mix:
- At least 2 queries explicitly aimed at SOURCE TEXT: GitHub repos (site:github.com + repo or path), "raw" README or main.go style hits,
  pkg.go.dev package pages, or official docs — not only high-level "what is X" articles.
- 1-2 queries for comparisons, landscape, or criticism if relevant.

Topic / rough notes:
{{.draft}}

Detected post type: {{.post_type}}
Core claim: {{.core_claim}}

Return JSON only, matching this schema:
{{.schema}}`

const promptResearchSynthesize = `You skim WEB SEARCH RESULTS for an upcoming blog: produce a compact orientation pack.
This is NOT the final evidence layer — section drafting uses an agent that may call search again for specifics.

Rules:
- overview: 2-3 paragraphs that foreground what a practitioner needs to decide or debug (constraints, common pitfalls, version/ecosystem deltas),
  not a generic "what is X" essay. Prefer "what breaks / what changed / what people trip on" over slogan-level positioning.
- evidence_items: 8-15 items. Each has a "fact" (2-3 sentences: the takeaway + what the source covers)
  and a "url" whenever the search results provide one — copy it verbatim. Prefer facts that name APIs, CLI flags, config keys,
  types, or file paths over vague "X integrates with Y". If the hit is mostly marketing, the fact should say what is still unknown.
  Do NOT invent URLs.
- key_resources: 8-14 notable links. Every item MUST include "url" copied verbatim from the results and a short "title".
  These URLs are what the blog will cite inline — maximize coverage of official docs, repos, and authoritative posts.
- verbatim_snippets: as many as needed up to 25. For EVERY markdown fenced code block in the search text
  (triple-backtick blocks) except "mermaid", you MUST emit one item: copy the inner code character-for-character,
  set "language" from the fence tag (or "text" if none), set "source_url" to the URL of that search hit if visible
  in the chunk (otherwise ""). Optional "note".
  ALSO emit items for multi-line code-like blocks in the search text that lack fences but are clearly listings
  (e.g. lines starting with "package ", "import ", "func ", shell lines with export/go get, or YAML keys) when they
  appear under a search hit — wrap the copied lines in the "snippet" field only (no extra backticks in JSON), set language sensibly.
  If the search shows only prose with no copyable listing, omit snippets — never invent code.
  NEVER paraphrase, translate, or "fix" code. NEVER fabricate snippets.

Topic context:
{{.draft}}

--- WEB SEARCH RESULTS ---
{{.search}}
--- END ---

Return JSON only, matching this schema:
{{.schema}}`

const promptDesignBlueprint = `You are a creative editorial architect for technical blog posts.

From the draft notes, detected analysis, and any knowledge base below, design a Blueprint — the full
creative structure for the post. Each section is drafted by an agent that **searches and fetches while writing** (not a pre-baked digest).

HARD RULES:
- 5-8 sections. Each section has a type: "narrative", "how-to", "comparison", "deep-dive", "aside", "conclusion".
- Each section's "purpose" must state a concrete reader outcome (e.g. "reader can choose A vs B given constraints X",
  "reader recognizes failure mode Y from logs") — not awareness-raising or vague "explores the topic".
- VARY section types. A good post mixes them: narrative → how-to → aside → deep-dive → comparison → conclusion.
  NEVER produce more than 2 consecutive sections of the same type.
- Plan artifacts explicitly. Each section lists what it needs: "mermaid-architecture", "mermaid-flow",
  "mermaid-sequence", "code-go", "code-bash", "code-yaml", "code-json", "table", or none.
  MINIMUM across the whole post: at least 2 mermaid diagrams.
  Code artifacts ("code-*"): when the Knowledge base or VERBATIM SNIPPETS contain a matching listing, plan code-* accordingly.
  When the Knowledge base is "(none)" / outline-only, assign code-* when the draft clearly implies code (named repo, language,
  CLI, config) — section writers will obtain exact text via DuckDuckGo_Search + Fetch_Page_Text during that section.
  If the topic has no plausible code, skip code-* for that section.
  Planned mermaid diagrams must be drawable: no literal "\\n" for line breaks in labels (use <br/>
  or markdown-string newlines per MERMAID rules in the writing stage).
- Creative headings. NOT "Step 1: Install X" or "Introduction". Use headings that reveal the section's
  angle: "Getting the pieces on the board", "Where things go sideways", "What nobody tells you about X".
- narrative_arc: 1-2 sentences describing the post's emotional/intellectual journey.
  Example: "Start with the frustration of configuring by hand, build toward the clarity of the graph model,
  end with honest gaps."
- The FIRST section should be type "narrative" — it's the hook, not a TL;DR or bullet list.
- The LAST section should be type "conclusion" — honest, opinionated, forward-looking.

LINK CITATION PLAN:
- When the Knowledge base has URLs (Key Resources / Evidence / Raw web search): each section SHOULD include "cite_urls" with 1-4 URLs
  copied EXACTLY from that material. Prefer github.com/.../blob/... or raw.githubusercontent.com file URLs over /tree/ when you need code.
  With upfront prefetch (legacy batch mode), blob URLs can be prefetched for verbatim fences.
- OUTLINE-ONLY MODE (Knowledge base is "(none)" or empty): **cite_urls may be empty arrays [].** Do not invent URLs here.
  Instead each section MUST include rich "search_hints": 3-6 concrete queries a human would type mid-draft (name repos, error strings,
  site:github.com, pkg.go.dev paths, doc hostnames). Section agents will discover URLs and excerpts live while writing.
- When the draft notes already contain explicit https URLs, you MAY copy those verbatim into cite_urls for the relevant section(s).
- Each section needs either non-empty cite_urls (when KB had sources) OR strong search_hints (outline-only).

Draft notes:
{{.draft}}

Post type: {{.post_type}}
Audience: {{.audience}}
Core claim: {{.core_claim}}

Knowledge base (from web research):
{{.knowledge_base}}

VERBATIM SNIPPETS (structured copies from research JSON — use together with Raw web search in the Knowledge base):
{{.verbatim_snippets}}

{{.must_include_block}}

Return JSON only, matching this schema:
{{.schema}}`

const promptSectionAgentSystem = `You draft ONE section of a technical blog in Markdown.

You have the DuckDuckGo_Search tool (and may have Fetch_Page_Text). Write like a human author: sketch the argument, then **search and fetch
as you go** when you need a link, a quote, or a code listing — not as if the whole article were researched beforehand.
Call tools mid-composition whenever you need URLs, documentation, or verbatim fences from the web.
When you have enough material, STOP calling tools and write the section.

FINAL assistant message rules (no tool calls in that message):
- Output ONLY the section markdown. The first line must be the "## …" heading exactly as specified in the task.
- No preamble, no narration about searching, no "Here is the section".

` + mermaidSyntaxRules + `

` + verbatimExcerptEthics + `

` + verbatimCopyRules + `

` + codeGroundingRules + `

` + codeSourceLineRules + `

` + citationRules + `

` + substantiveBlogRules + `

FENCE WRAPPING:
- If search or KB shows code without a fenced block, add triple-backtick open (with language) / close only around that text — the enclosed
  text must still be a character-accurate copy. Search again until you have a concrete listing to cite.

TOOLS:
- DuckDuckGo_Search: discovery and page snippets.
- When available, Fetch_Page_Text: given a GitHub https://…/blob/branch/path/file URL (from cite plan or search), fetch RAW file text.
  Use this before pasting a Go/bash/json listing so the tool output is your authoritative paste source. Plain marketing pages are not substitutes.

STYLE:
- No filler ("In today's world", "It is worth noting", "game-changer").
- Prefer "I found that…" over "It has been observed that…".
- Do not open by restating the heading as a full sentence; vary openings — ideally open with the sharpest concrete detail you can cite.
- Banned vapour: "In today's world", "delve", "landscape" (overuse), "leverage" (overuse), "robust" without specifics,
  "unlock", "game-changer", "synergy", "paradigm" unless quoted. Prefer one concrete fact with URL over three vague sentences.
- Self-check before finishing: if someone skimming removed every sentence without a proper noun, URL, code token, or number, would anything remain? If yes, cut or replace those lines.

` + authorColorRules

const promptSectionMissingCodeRetry = `RETRY — blueprint requires a code-* artifact but your draft has no non-mermaid fenced listing yet.
- If "### Full file text (verbatim source" appears above, copy a contiguous span from that file into ONE fenced block with the correct
  language tag (go, bash, yaml, …). The fenced inner text must match the file bytes exactly (only elision lines allowed per COPYING CODE rules).
  The next line after the closing fence must be **Code source:** [title](url) with the SAME url as in that prefetch header.
- If there is no prefetch for this section, call Fetch_Page_Text on a github.com/.../blob/... URL from the cite plan, then quote from tool output only.
- Output only the section markdown starting with the ## heading; no meta.`

const promptSectionThinRetry = `RETRY — the last draft was too thin, empty after checks, or lacked grounding.
- Target at least ~450 words of substantive prose in this section (not counting mermaid or code fences).
- Every non-trivial claim needs a markdown link from cite_urls, Key Resources, or a new tool search.
- If this section requires code-* artifacts: call Fetch_Page_Text on a GitHub blob URL from the cite plan (or find one via search),
  then use ONLY verbatim listings from Fetch_Page_Text or DuckDuckGo_Search output inside fences — no invented APIs.
- Personality and opinions belong in prose only; fences stay byte-for-byte from the tool output (see EXCERPTS ARE EVIDENCE).
- Output only the section markdown starting with the ## heading; no apology or meta about this retry.`

const promptPublishPolish = `You are a senior technical editor publishing a post in ONE pass (structure + citations + personality in prose).

` + verbatimExcerptEthics + `

` + authorColorRules + `

Voice to channel: {{.voice}}

Knowledge base (use ONLY these URLs if you add or fix citations — never invent links):
{{.knowledge_base}}

VERBATIM SNIPPETS (optional structured snippets; Raw web search in the Knowledge base is also authoritative):
{{.verbatim_snippets}}

Editorial + voice (single pass — avoid a second rewrite that drifts code):
- Fix heading hierarchy (single # title, ## sections, ### subsections). Do NOT change section headings' meaning.
- Remove accidental duplicates, contradictions, or repeated phrases across sections.
- In prose between code blocks: add or sharpen personality — short asides, a vivid analogy, honest "I'd watch for…" lines,
  and warmer transitions where the draft is stiff. Aim for a few memorable lines per major section without turning the post into comedy.
  Texture must not replace technical density: do not swap procedures, cited facts, or quoted code for tone; do not pad length.
- Ensure every mermaid block uses triple-backtick "mermaid" fencing.
- Fix broken Mermaid: remove literal "\\n" from inside node/edge labels where a line break was
  intended — replace with <br/> or restructure using markdown string nodes with real newlines.
  Fix "end" in node text (capitalize). Match opening/closing subgraph and fence.
- Ensure every non-mermaid code block specifies a language.
- Never rewrite characters inside non-mermaid fenced bodies — preserve inner text byte-for-byte if kept.
  Do NOT add NEW application (non-mermaid) fences.
- Every non-mermaid fence MUST be immediately followed by **Code source:** [title](url). If missing, add it using ONLY
  URLs from the Knowledge base; if impossible, remove the fence.
- A **Code source:** URL is NOT permission to paste code. The fence inner text MUST appear verbatim in the Knowledge base
  (especially "### Raw web search") or VERBATIM SNIPPETS — character-for-character after trailing-space trim per line.
  Wiki pages that only reference filenames do not contain file bodies; never fabricate listings. If the listing is not in the KB/verbatim text,
  remove the fence and link to the repo file instead.
- Preserve fence bodies that match sources; remove unfounded fences; do not invent replacement code.
- Preserve ALL markdown links [text](url) exactly — same URLs. Do not drop or fabricate links.
- Citation density: if the article cites fewer than 8 distinct URLs in the body, add natural [text](url) links
  where sentences support claims, using ONLY URLs from the Knowledge base above.
- Remove filler: "In today's world", "It is worth noting", "In conclusion", "indispensable", "game-changer", "at the end of the day".
- Cut or tighten paragraphs that only restate headings or list generic benefits without cited facts or links.
- Strengthen topic sentences where sections are mushy. If two section openings share the same rhetorical pattern, rewrite one.
- Do NOT pad. If it's thin, leave it.

--- BEGIN POST ---
{{.post}}
--- END POST ---

Output only the final markdown.`

const promptPublishPolishSection = `You polish ONE section of a technical blog (Markdown fragment only).

` + verbatimExcerptEthics + `

` + authorColorRules + `

Voice to channel: {{.voice}}

{{.section_context}}

Knowledge base (use ONLY these URLs if you add or fix citations — never invent links):
{{.knowledge_base}}

VERBATIM SNIPPETS:
{{.verbatim_snippets}}

Section-scoped rules (full article is NOT in context — use the outline + prior tail above for coherence):
- Output ONLY this section. The first line MUST be the exact same "## …" heading as in the fragment below.
- Apply mermaid fix rules, code fence / **Code source:** rules, and voice-in-prose like the full-document editor.
- Stay aligned with the narrative arc and outline: do not repeat the same hook, metaphor, or opening gimmick you see in the
  previous section's tail; vary sentence rhythms across the post. Do not re-introduce topics that clearly belong to another section.
- Do not aggressively shorten: keep technical detail, examples, and cite-worthy claims unless they are clearly filler.
- Never rewrite non-mermaid fence bodies; do not add new application fences.
- Preserve all [text](url) unless fixing anchor wording; never invent URLs.

--- BEGIN SECTION ---
{{.section_body}}
--- END SECTION ---

Output only the section markdown.`

func buildSectionAgentUserContent(
	spec SectionSpec, blueprint Blueprint, kb, verbatim, prior, voice, draft string,
) string {
	kb = strings.TrimSpace(kb)
	if kb == "" {
		kb = "(empty — rely on DuckDuckGo_Search for this section.)"
	}
	vBlock := strings.TrimSpace(verbatim)
	if vBlock == "" {
		vBlock = "(none — when you need code, search and copy from results or Raw web search in the excerpt below.)"
	}
	return fmt.Sprintf(`Write this single section. Use DuckDuckGo_Search for discovery; when cite URLs include GitHub file links, use Fetch_Page_Text to load raw source before pasting code. Reply with ONLY the section markdown.

Section heading (first line of your final answer, exactly): ## %s

SECTION SPEC:
- Type: %s
- Purpose: %s
- Position in narrative arc: %s

%s

%s

%s

ARTICLE CONTEXT:
- Title: %s
- Thesis: %s
- Audience: %s
- Post type: %s
- Voice: %s

PRIOR SECTIONS (continuity — do not repeat):
%s

Knowledge base excerpt (initial skim; search again when you need live code or URLs):
%s

VERBATIM SNIPPETS from initial research (optional structured copies):
%s

Original rough draft (do not contradict):
%s

URLs to prioritize for deep fetch (GitHub blob → raw; use Fetch_Page_Text before fenced code when applicable):
%s

OUTPUT: When finished researching, your entire reply must be the section only — start with "## %s" then the body.`,
		spec.Title,
		spec.Type,
		spec.Purpose,
		blueprint.NarrativeArc,
		sectionTypeInstructions(spec.Type),
		artifactInstructions(spec.Artifacts),
		buildCitationBlock(spec),
		blueprint.Title,
		blueprint.Thesis,
		blueprint.Audience,
		blueprint.PostType,
		voice,
		prior,
		kb,
		vBlock,
		draft,
		citeURLsFetchHints(spec),
		spec.Title,
	)
}

func citeURLsFetchHints(spec SectionSpec) string {
	if len(spec.CiteURLs) == 0 {
		return "(none — discover github.com/.../blob/... or raw.githubusercontent.com links via search.)"
	}
	var b strings.Builder
	for _, u := range spec.CiteURLs {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(u)
		b.WriteByte('\n')
	}
	s := strings.TrimSpace(b.String())
	if s == "" {
		return "(none)"
	}
	return s
}

func sectionTypeInstructions(stype string) string {
	switch stype {
	case "narrative":
		return `TYPE INSTRUCTIONS (narrative):
- Story-driven: set the scene, use an analogy or concrete scenario.
- No bullet lists in this section — flowing prose only.
- 400-700 words.
- This section should make the reader care about the problem before jumping to solutions.
- Lean into voice here (wit, impatience with bad defaults, a vivid metaphor) — still no invented facts; cite when naming real projects or behavior.
- Cite 2+ sources inline when you mention tools, papers, or ecosystems (see INLINE CITATIONS).`
	case "how-to":
		return `TYPE INSTRUCTIONS (how-to):
- EXACT-COPY only from DuckDuckGo_Search results or the KB/VERBATIM excerpts per VERBATIM COPY. If the hit is unfenced shell/go,
  add bash or go fenced blocks (three backticks) yourself around the copied lines only.
- When artifacts include code-*, include at least one such fence with **Code source:** if a matching block exists;
  otherwise prose + links.
- Include at least one gotcha or "watch out for this" note.
- 500-800 words.
- Numbered steps or clear sequential flow, but weave explanation between them.
- If search turns up no usable fence, do not invent code.`
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
- Any quoted implementation code must be an EXACT copy from DuckDuckGo_Search or KB/VERBATIM — no rewrite; you may add triple-backtick
  fences if the source omitted them.
- Teach, don't just describe. Help the reader build a mental model.
- 500-900 words.
- Cite design docs, specs, or source repos inline when you explain behavior.`
	case "aside":
		return `TYPE INSTRUCTIONS (aside):
- Short personal note, opinion, or anecdote. Conversational tone.
- This is where personality lives: share what surprised you, what frustrated you, or what you'd do differently — one honest opinion beats generic praise.
- If you quote code here, it is still subject to EXCERPTS ARE EVIDENCE; otherwise stay in prose.
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
				"- Required: one ```%s block — inner text verbatim from a search hit or KB snippet; you may add the ```%s delimiters "+
					"if the source was unfenced. Then **Code source:**. If no listing exists, prose + links only.", lang, lang))
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
		return "SECTION CITATION PLAN: No pre-placed cite_urls — discover sources with DuckDuckGo_Search (and Fetch_Page_Text for GitHub file URLs) " +
			"while drafting, then meet INLINE CITATIONS using only URLs that appeared in tool output this section."
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
