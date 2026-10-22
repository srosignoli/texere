# Audio cell — Phase 2 (STT) Manual Test Plan

Prereqs: open an extract; have an OpenAI-compatible transcription endpoint
reachable (OpenAI `whisper-1`, or a local whisper server on `http://localhost:8080/v1`).

## TC-1 Configure an STT profile
1. Open ⚙ Models; click the "Speech-to-text" tab.
   - EXPECT: an empty STT list with presets "OpenAI Whisper" and "Local whisper"; no Kind selector in the editor; no Test button; the key field says "optional".
2. Add a preset, fill the key if needed, Save; mark it active.
   - EXPECT: it appears in the STT list and persists across reopening the panel.

## TC-2 Transcribe
1. On an audio cell that has a clip, pick the STT model and click Transcribe.
   - EXPECT: a "Transcribing…" state, then a new N4L draft cell appears below with the transcript text.

## TC-3 Model picker + keyless local
1. Add a keyless "Local whisper" profile; Transcribe with it.
   - EXPECT: works with no API key (no auth sent); the request hits the local endpoint.

## TC-4 Errors
1. Transcribe with a bad Base URL / model.
   - EXPECT: an inline error message; no draft inserted.
2. With no STT profiles configured, the Transcribe button is disabled with a hint.

## TC-5 LLM tab unaffected (regression)
1. Switch to the "LLM Providers" tab.
   - EXPECT: existing LLM profiles, the Kind selector, and the Test button all still work.
