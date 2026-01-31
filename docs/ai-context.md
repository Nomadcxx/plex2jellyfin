# AI Title Matching with Context

## Overview

JellyWatch uses local AI (Ollama) to parse media filenames and extract
clean metadata (title, year, season, episode). The AI receives contextual
information to improve accuracy and prevent hallucinations.

## Context Provided to AI

When analyzing a file, the AI receives:

1. **Library Type**: "movie library" or "TV show library"
   - Helps disambiguate titles (e.g., "The Office" could be movie or TV)
   - Influences confidence in file type detection

2. **Folder Path**: Parent directory name (sanitized)
   - May contain series name (e.g., "/Prison Break/Season 4/")
   - Helps identify correct show when filename is ambiguous

3. **Current Metadata**: Existing parse from database
   - Title that was previously extracted
   - Confidence score (low confidence < 0.8 indicates likely error)
   - Used as hint but not bound by it

4. **Filename**: The full release filename
   - Contains quality indicators, release groups, season/episode markers
   - Primary source of information

## Example

### Without Context (OLD BEHAVIOR):
```
Input: "pb.s04e15.720p.brrip.mkv"
AI Guesses: "History's Greatest Mysteries - S07E01" ❌ WRONG
```

### With Context (NEW BEHAVIOR):
```
Input:
- Filename: "pb.s04e15.720p.brrip.mkv"
- Library Type: "TV show library"
- Folder: "Prison Break"
- Current Title: "pb" (confidence: 0.65)

AI Guesses: "Prison Break - S04E15" ✅ CORRECT
```

## Validation

AI suggestions are validated through multiple layers:

1. **Library Path Analysis**: Check if AI's suggested type matches folder location
2. **Sonarr/Radarr Lookup**: Query APIs to confirm title existence
3. **Confidence Threshold**: Reject low-confidence suggestions (< 0.8)

## Configuration

AI matching is controlled via `~/.config/jellywatch/config.toml`:

```toml
[ai]
enabled = true
ollama_endpoint = "http://localhost:11434"
model = "llama3.1"
confidence_threshold = 0.8
timeout_seconds = 30
```

## Troubleshooting

### AI Still Hallucinating?

1. Enable debug mode: `export DEBUG_AI=1`
2. Check what context is being sent: Look for "File Context" in prompts
3. Verify folder structure is clean (no obfuscated names)
4. Increase `confidence_threshold` to reject uncertain suggestions

### Wrong Media Type?

If AI suggests movie for TV show or vice versa:
- Check folder path (should be in correct library)
- Verify library root config in config.toml
- Run `jellywatch scan` to update database
