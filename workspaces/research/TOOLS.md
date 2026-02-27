# Research Scout - Tools

## FOSS Skills Available

### arXiv Search
```bash
/home/node/skills/arxiv-fetcher/search.py "LLM reasoning"
```
Returns: arXiv API URL → use `web_fetch` to retrieve XML

### Semantic Scholar
```bash
/home/node/skills/arxiv-fetcher/enhanced_search.py "GraphRAG"
```
Returns: JSON with papers, citations, abstracts

### SearXNG (Web Search)
```bash
/home/node/skills/searxng/search.py "latest AI research"
```
Returns: Web search results (for finding blog posts about papers)

## Tool Preferences

**For morning scans**: Use arXiv API directly (faster, authoritative)
**For specific topics**: Combine arXiv + Semantic Scholar (broader coverage)
**For context**: Use SearXNG to find blog posts explaining papers

## Memory Files You Maintain

- `memory/research-YYYY-MM-DD.md` - Daily scan results
- `memory/tomorrow/research.md` - Specific prep for tomorrow
- `memory/MEMORY.md` - Long-term research trends

## Citation Format
Always use: `[Paper Title] (arXiv:XXXX.XXXXX)` or `[Paper Title] (S2: semantic-scholar-id)`
