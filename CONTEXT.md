# Texere

A desktop workbench for writing, exploring and studying knowledge graphs written in N4L.

## Language

**Collection**:
A folder of `.n4l` files together with everything Texere derives and keeps inside it: the graph, the search index, study state, media and reading sources.
_Avoid_: Workspace, vault, project

**Session**:
One Collection while it is open in Texere, from opening to closing. At most one Session exists at a time, and switching Collections ends one Session and starts another.
_Avoid_: Open collection, workspace

### Notebooks

**Notebook**:
The cell-based workspace attached to one node, where that node is worked into N4L. A node has at most one Notebook.
_Avoid_: Notebook link, scratchpad

**Push**:
Inserting a Notebook's draft into the note's `.n4l` file.
_Avoid_: Export, publish

### Study

**Study Element**:
Anything the study queue can put in front of the learner: a Source, an Extract or a Card.
_Avoid_: Queue item, element (unqualified)

**Source**:
A document being read incrementally inside a Collection, resumed where the learner left off.
_Avoid_: Reading element, reading, document

**Extract**:
A passage pulled out of a Source, waiting to be processed into Cards or dismissed.
_Avoid_: Clip, highlight, snippet

**Card**:
One recall prompt whose next review is scheduled by FSRS.
_Avoid_: Item, flashcard

**Topic**:
A `.n4l` note considered as something to curate. Not yet a Study Element.
_Avoid_: Note (when curation is meant), chapter

**Sitting**:
One uninterrupted stretch of reading a Source, from opening it to closing it. A peek, which is opening a Source at one passage just to look at it, is a Sitting that records nothing.
_Avoid_: Reading session, reading

**Turn**:
One Study Element served by the queue, from the moment it is shown until the learner finishes with it; a Source's Turn ends when it is closed or marked done.
_Avoid_: Step, review (for non-Cards)

**Mix**:
The target share of Turns given to each kind of Study Element, set per Collection. A share of zero means that kind is never served.
_Avoid_: Ratio, weights, quota

**Study Queue**:
What decides the next Turn, by serving whichever kind of Study Element is furthest below its Mix share in the current Session.
_Avoid_: Cascade, priority queue (until priorities exist)
