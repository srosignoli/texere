Texere is a personal knowledge management and learning platform based on Mark Burgess's [SSTorytime](https://github.com/markburgess/SSTorytime/) and [N4L](https://github.com/markburgess/SSTorytime/blob/main/docs/N4L.md), combined with Piotr Woźniak's concepts of [incremental reading](https://help.supermemo.org/wiki/Incremental_learning) and [spaced repetition](https://supermemo.guru/wiki/Spaced_repetition).

## Local and Portable

The tool is designed to run locally on your computer, supporting Windows, Linux, and macOS. 

SSTorytime has been ported to Go([SSTorytime-KV](https://github.com/srosignoli/sstorytime-kv)) so it runs directly within Texere. All your notes and knowledge stay in plain text files inside a collection folder. Scheduling data for spaced repetition and incremental reading is stored in a local SQLite database within that same collection. 

To move your notes to a different computer, simply zip the collection folder. Everything works immediately with the Texere instance installed there—no database servers, cloud lock-in, or external setup required.

You can download example N4L collections from the original [SSTorytime](https://github.com/markburgess/SSTorytime/tree/main/examples) repository or from my [SSTorytime-KV](https://github.com/srosignoli/sstorytime-kv/tree/main/examples) port.

## AI Disclaimer: How Texere Was Built

This project was developed through [automated | augmented | agentinc] programming (select whichever term is currently trending on LinkedIn today). One of my goals was to gain practical experience with agents: define the core ideas, specify the features, and let AI agents handle the implementation.

This is not the result of a single prompt. It is a months long journey exploring different approaches, all documented in the repository's `docs` folder. 

I started with spec-driven development. Later, I moved to the Superpowers skill workflow:

1. **Planning phase:** Explore and plan features using the most capable model available on my subscription.
2. **Implementation phase:** Hand over tasks to agents running lighter, faster models.
3. **Review phase:** Perform code and spec reviews using the top-tier model.

This strategy is currently my preferred approach, though it continues to evolve. The workflow depends on the model: newer models need less steering and prompting. It also depends on token limits. Token budgets are limited for me, I use entry level flat subscription, so I cannot afford to develop exclusively with the most expensive model at all times.

I wrote the README, documentation, and blog articles myself, editing and polishing them with [my voice skill](https://github.com/srosignoli/agent-skills/tree/main/skills/voice-editor).

## References and Further Reading

For more details on the foundational concepts combined in Texere, here are the primary starting points from the original authors:

- **[Mark Burgess](https://markburgess.org/):** *[SSTorytime](https://github.com/markburgess/SSTorytime/)* and *[Notes for Learning (N4L)](https://github.com/markburgess/SSTorytime/blob/main/docs/N4L.md)* — exploring associative knowledge graphs, narrative structures, and semantic arrows.
- **[Piotr Woźniak](https://supermemo.guru/wiki/SuperMemo_Guru):** *[Incremental Reading](https://help.supermemo.org/wiki/Incremental_learning)* and *[Spaced Repetition](https://supermemo.guru/wiki/Spaced_repetition)* (SuperMemo) — the workflow of reading in small bites, extracting key points, and scheduling reviews with spaced repetition.

## Texere Tutorials

- **[Texere Tutorials](https://www.simonerosignoli.eu/texere/):** 
    - [Texere Introduction: Weaving Knowledge Instead of Collecting Notes](https://www.simonerosignoli.eu/texere/texere-introduction/) 
    - [Texere: Writing a Note](https://www.simonerosignoli.eu/texere/texere-writing-a-note/) 
    - [Texere: Three-Column Navigation Layout](https://www.simonerosignoli.eu/texere/texere-3-columns/) 
    - [Texere: The Preview Panel](https://www.simonerosignoli.eu/texere/texere-the-preview-panel/) 
    - [Texere: Building the Graph](https://www.simonerosignoli.eu/texere/texere-build-graph/) 
    - [Texere: Fuzzy Search](https://www.simonerosignoli.eu/texere/texere-fuzzy-search/) 
    - [Texere: The Graph Query Language](https://www.simonerosignoli.eu/texere/texere-graph-query-language/) 
    - [Texere: Incremental Reading](https://www.simonerosignoli.eu/texere/texere-reading/) 
    - [Texere: Processing Extracts into Knowledge](https://www.simonerosignoli.eu/texere/texere-processing/) 
    - [Texere: The Context Cell](https://www.simonerosignoli.eu/texere/texere-context-cell/) 
    - [Texere: The N4L Draft Cell](https://www.simonerosignoli.eu/texere/texere-n4l-draft-cell/) 
    - [Texere: The Graph Cell](https://www.simonerosignoli.eu/texere/texere-graph-cell/) 
    - [Texere: The Sketch Cell](https://www.simonerosignoli.eu/texere/texere-sketch-cell/) 
    - [Texere: The Media Cell](https://www.simonerosignoli.eu/texere/texere-media-cell/) 
    - [Texere: The Shell Cell](https://www.simonerosignoli.eu/texere/texere-shell-cell/) 
    - [Texere: The LLM Cell](https://www.simonerosignoli.eu/texere/texere-llm-cell/) 
    - [Texere: The Scan Cell](https://www.simonerosignoli.eu/texere/texere-scan-cell/) 
    - [Texere: The Audio Cell](https://www.simonerosignoli.eu/texere/texere-audio-cell/) 
    - [Texere: The Compute Cell](https://www.simonerosignoli.eu/texere/texere-compute-cell/) 
    - [Texere: Organizing the Notebook](https://www.simonerosignoli.eu/texere/texere-organizing-the-notebook/) 
    - [Texere: Notebooks as Margin Notes](https://www.simonerosignoli.eu/texere/texere-notebooks-as-margin-notes/) 
    - [Texere: Writing Flashcards: Cards and Clozes](https://www.simonerosignoli.eu/texere/texere-flashcards-and-clozes/) 
    - [Texere: Grading Cards and Review Scheduling](https://www.simonerosignoli.eu/texere/texere-grading-and-scheduling/) 
    - [Texere: The Stats Panel](https://www.simonerosignoli.eu/texere/texere-stats-panel/) 
    - [Texere: The Settings Menu](https://www.simonerosignoli.eu/texere/texere-settings-menu/) 
    - [Texere: Next: Just Keep Going](https://www.simonerosignoli.eu/texere/texere-next-workflow/) 

## Video Tutorial

For a visual tour of Texere and its core features, watch the overview video:

[![Watch the video](https://img.youtube.com/vi/JWcg9q0KCpw/maxresdefault.jpg)](https://www.youtube.com/watch?v=JWcg9q0KCpw)


## Install

Download the archive for your platform from
[Releases](https://github.com/srosignoli/texere/releases) and check it against
`checksums.txt`.

- **Linux** — `texere_<version>_linux_amd64.tar.gz`. Needs
  `libwebkit2gtk-4.1-0` and `libgtk-3-0`.
- **Windows** — `texere_<version>_windows_amd64.zip`. Single executable.

## Build from source

```bash
make deps        # Linux system libraries
make wails-cli   # Wails CLI, built with the toolchain go.mod asks for
make build       # → build/bin/texere
make test        # go test ./...
```

Full setup, cross-compilation and the test matrix are in
[docs/quickstart.md](docs/quickstart.md).

## Documentation

| | |
|---|---|
| [docs/quickstart.md](docs/quickstart.md) | Developer setup, build, test, project layout |
| [docs/query-language.md](docs/query-language.md) | Query syntax and the ask box |
| [docs/Texere_Specification_v1.3.md](docs/Texere_Specification_v1.3.md) | The long-form design document. Aspirational — much of it is unbuilt, and ROADMAP.md says which parts are queued |
| [docs/specs/](docs/specs/), [docs/plans/](docs/plans/) | Design specs and implementation plans, one per feature or change |
| [docs/principles.md](docs/principles.md) | The project's founding principles: plain-text N4L as the source of truth, one self-contained binary, … |

## Licence

Apache License 2.0
