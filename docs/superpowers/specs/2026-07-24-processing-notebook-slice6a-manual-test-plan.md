# Processing Notebook — Slice 6a (Multimodal) Manual Test Plan

Prereqs: open an extract; configure at least one vision-capable LLM profile (e.g. an Anthropic Claude profile) in ⚙ Models.

## TC-1 Attach a sketch to an LLM cell
1. Add a Sketch cell; draw a simple shape. Add an LLM cell.
2. In the LLM cell's attach row, check the sketch; ask "what did I draw?".
   - EXPECT: the model answers describing the drawing.

## TC-2 Attach an image
1. Add an Image cell; import a photo. Attach it to an LLM cell; ask about it.
   - EXPECT: the model answers about the photo (rotation/crop are reflected).

## TC-3 Transcribe handwriting → N4L draft
1. On a sketch of handwritten words, pick a model, click Transcribe.
   - EXPECT: a new N4L draft cell appears directly below with the transcribed text.

## TC-4 Transcribe a diagram → Mermaid
1. Draw/photograph a small flowchart; Transcribe.
   - EXPECT: the draft contains a ```mermaid code block approximating the diagram.

## TC-5 Model picker
1. Change the dropdown to a different profile; Transcribe again.
   - EXPECT: the request uses the selected model (different result/latency).

## TC-6 Errors
1. Transcribe with a non-vision model.
   - EXPECT: an error message; no draft inserted.
2. Text-only LLM chat still works (attach nothing; converse) — regression check.

## TC-7 Persistence
1. Attach a sketch, chat, restart the app, reopen.
   - EXPECT: the conversation persists; no image bytes bloat (the notebook still opens fast); re-asking re-bakes the image.
