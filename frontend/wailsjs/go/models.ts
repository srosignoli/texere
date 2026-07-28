export namespace content {
	
	export class Arrow {
	    Code: string;
	    Target: string;
	    IsRegistered: boolean;
	    Line: number;
	    IsDitto: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Arrow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Code = source["Code"];
	        this.Target = source["Target"];
	        this.IsRegistered = source["IsRegistered"];
	        this.Line = source["Line"];
	        this.IsDitto = source["IsDitto"];
	    }
	}
	export class ArrowDef {
	    Code: string;
	    LongName: string;
	    Reverse: string;
	    MetaType: string;
	    Context: string;
	    ClozeEligible: boolean;
	    Unknown: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ArrowDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Code = source["Code"];
	        this.LongName = source["LongName"];
	        this.Reverse = source["Reverse"];
	        this.MetaType = source["MetaType"];
	        this.Context = source["Context"];
	        this.ClozeEligible = source["ClozeEligible"];
	        this.Unknown = source["Unknown"];
	    }
	}
	export class Provenance {
	    Source: string;
	    Ref: string;
	    Progress: string;
	
	    static createFrom(source: any = {}) {
	        return new Provenance(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Source = source["Source"];
	        this.Ref = source["Ref"];
	        this.Progress = source["Progress"];
	    }
	}
	export class StudyMeta {
	    CardType: number;
	    ForceReverse: boolean;
	    SuppressReverse: boolean;
	    ClozeTargets: string[];
	    FromBlock: boolean;
	    BlockType: number;
	
	    static createFrom(source: any = {}) {
	        return new StudyMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.CardType = source["CardType"];
	        this.ForceReverse = source["ForceReverse"];
	        this.SuppressReverse = source["SuppressReverse"];
	        this.ClozeTargets = source["ClozeTargets"];
	        this.FromBlock = source["FromBlock"];
	        this.BlockType = source["BlockType"];
	    }
	}
	export class DittoLine {
	    Line: number;
	    Text: string;
	    Arrows: Arrow[];
	
	    static createFrom(source: any = {}) {
	        return new DittoLine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Line = source["Line"];
	        this.Text = source["Text"];
	        this.Arrows = this.convertValues(source["Arrows"], Arrow);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Node {
	    ID: string;
	    Text: string;
	    Line: number;
	    EffectiveCtx: string[];
	    IsSequence: boolean;
	    Arrows: Arrow[];
	    Dittos: DittoLine[];
	    StudyMeta?: StudyMeta;
	    Priority?: number;
	    Provenance?: Provenance;
	    Label: string;
	    IDMarkerLine: number;
	
	    static createFrom(source: any = {}) {
	        return new Node(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Text = source["Text"];
	        this.Line = source["Line"];
	        this.EffectiveCtx = source["EffectiveCtx"];
	        this.IsSequence = source["IsSequence"];
	        this.Arrows = this.convertValues(source["Arrows"], Arrow);
	        this.Dittos = this.convertValues(source["Dittos"], DittoLine);
	        this.StudyMeta = this.convertValues(source["StudyMeta"], StudyMeta);
	        this.Priority = source["Priority"];
	        this.Provenance = this.convertValues(source["Provenance"], Provenance);
	        this.Label = source["Label"];
	        this.IDMarkerLine = source["IDMarkerLine"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Chapter {
	    Title: string;
	    LineStart: number;
	    Nodes: Node[];
	    Labels: Record<string, Node>;
	
	    static createFrom(source: any = {}) {
	        return new Chapter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Title = source["Title"];
	        this.LineStart = source["LineStart"];
	        this.Nodes = this.convertValues(source["Nodes"], Node);
	        this.Labels = this.convertValues(source["Labels"], Node, true);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ChapterSummary {
	    FilePath: string;
	    FileName: string;
	    RelPath: string;
	    Chapters: string[];
	    ScanError: any;
	
	    static createFrom(source: any = {}) {
	        return new ChapterSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.FilePath = source["FilePath"];
	        this.FileName = source["FileName"];
	        this.RelPath = source["RelPath"];
	        this.Chapters = source["Chapters"];
	        this.ScanError = source["ScanError"];
	    }
	}
	
	export class IDInjection {
	    FilePath: string;
	    NodeLine: number;
	    InsertedAt: number;
	    ShortID: string;
	    AlreadyHad: boolean;
	
	    static createFrom(source: any = {}) {
	        return new IDInjection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.FilePath = source["FilePath"];
	        this.NodeLine = source["NodeLine"];
	        this.InsertedAt = source["InsertedAt"];
	        this.ShortID = source["ShortID"];
	        this.AlreadyHad = source["AlreadyHad"];
	    }
	}
	
	export class ParseError {
	    File: string;
	    Line: number;
	    Msg: string;
	
	    static createFrom(source: any = {}) {
	        return new ParseError(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.File = source["File"];
	        this.Line = source["Line"];
	        this.Msg = source["Msg"];
	    }
	}
	export class Reminder {
	    File: string;
	    Line: number;
	    Text: string;
	
	    static createFrom(source: any = {}) {
	        return new Reminder(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.File = source["File"];
	        this.Line = source["Line"];
	        this.Text = source["Text"];
	    }
	}
	export class ParsedFile {
	    Path: string;
	    Chapters: Chapter[];
	    Reminders: Reminder[];
	    Errors: ParseError[];
	
	    static createFrom(source: any = {}) {
	        return new ParsedFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Path = source["Path"];
	        this.Chapters = this.convertValues(source["Chapters"], Chapter);
	        this.Reminders = this.convertValues(source["Reminders"], Reminder);
	        this.Errors = this.convertValues(source["Errors"], ParseError);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	export class WriteBackResult {
	    FilePath: string;
	    LinesChanged: number[];
	    Err: any;
	
	    static createFrom(source: any = {}) {
	        return new WriteBackResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.FilePath = source["FilePath"];
	        this.LinesChanged = source["LinesChanged"];
	        this.Err = source["Err"];
	    }
	}

}

export namespace explorer {
	
	export class Diagnostics {
	    layoutMs: number;
	    clamped: boolean;
	    truncated: number;
	    graphError?: string;
	    matchCount: number;
	
	    static createFrom(source: any = {}) {
	        return new Diagnostics(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.layoutMs = source["layoutMs"];
	        this.clamped = source["clamped"];
	        this.truncated = source["truncated"];
	        this.graphError = source["graphError"];
	        this.matchCount = source["matchCount"];
	    }
	}
	export class EdgeView {
	    srcId: string;
	    dstId: string;
	    meta: string;
	    arrowLong: string;
	    crossesContexts: boolean;
	    unknown?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new EdgeView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.srcId = source["srcId"];
	        this.dstId = source["dstId"];
	        this.meta = source["meta"];
	        this.arrowLong = source["arrowLong"];
	        this.crossesContexts = source["crossesContexts"];
	        this.unknown = source["unknown"];
	    }
	}
	export class LayerView {
	    contextId: string;
	    label: string;
	    tags: string[];
	    chapters: string[];
	    y: number;
	    nodeCount: number;
	    colorSlot: number;
	    centerX: number;
	    centerZ: number;
	    extentX: number;
	    extentZ: number;
	
	    static createFrom(source: any = {}) {
	        return new LayerView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.contextId = source["contextId"];
	        this.label = source["label"];
	        this.tags = source["tags"];
	        this.chapters = source["chapters"];
	        this.y = source["y"];
	        this.nodeCount = source["nodeCount"];
	        this.colorSlot = source["colorSlot"];
	        this.centerX = source["centerX"];
	        this.centerZ = source["centerZ"];
	        this.extentX = source["extentX"];
	        this.extentZ = source["extentZ"];
	    }
	}
	export class LayoutBounds {
	    minX: number;
	    maxX: number;
	    minY: number;
	    maxY: number;
	    minZ: number;
	    maxZ: number;
	
	    static createFrom(source: any = {}) {
	        return new LayoutBounds(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.minX = source["minX"];
	        this.maxX = source["maxX"];
	        this.minY = source["minY"];
	        this.maxY = source["maxY"];
	        this.minZ = source["minZ"];
	        this.maxZ = source["maxZ"];
	    }
	}
	export class Vec2J {
	    x: number;
	    z: number;
	
	    static createFrom(source: any = {}) {
	        return new Vec2J(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.x = source["x"];
	        this.z = source["z"];
	    }
	}
	export class TerritoryView {
	    contextId: string;
	    vertices: Vec2J[];
	    colorSlot: number;
	
	    static createFrom(source: any = {}) {
	        return new TerritoryView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.contextId = source["contextId"];
	        this.vertices = this.convertValues(source["vertices"], Vec2J);
	        this.colorSlot = source["colorSlot"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NodeView {
	    id: string;
	    label: string;
	    x: number;
	    y: number;
	    z: number;
	    size: number;
	    contextId: string;
	    inDegree: number;
	    cycleWarning?: boolean;
	    distanceFromAnchor: number;
	
	    static createFrom(source: any = {}) {
	        return new NodeView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.x = source["x"];
	        this.y = source["y"];
	        this.z = source["z"];
	        this.size = source["size"];
	        this.contextId = source["contextId"];
	        this.inDegree = source["inDegree"];
	        this.cycleWarning = source["cycleWarning"];
	        this.distanceFromAnchor = source["distanceFromAnchor"];
	    }
	}
	export class LayoutResult {
	    resultId: string;
	    anchorId: string;
	    bounds: LayoutBounds;
	    nodes: NodeView[];
	    edges: EdgeView[];
	    layers: LayerView[];
	    territories: TerritoryView[];
	    diagnostics: Diagnostics;
	
	    static createFrom(source: any = {}) {
	        return new LayoutResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.resultId = source["resultId"];
	        this.anchorId = source["anchorId"];
	        this.bounds = this.convertValues(source["bounds"], LayoutBounds);
	        this.nodes = this.convertValues(source["nodes"], NodeView);
	        this.edges = this.convertValues(source["edges"], EdgeView);
	        this.layers = this.convertValues(source["layers"], LayerView);
	        this.territories = this.convertValues(source["territories"], TerritoryView);
	        this.diagnostics = this.convertValues(source["diagnostics"], Diagnostics);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	

}

export namespace learning {
	
	export class CardView {
	    id: string;
	    front: string;
	    back: string;
	    cardType: string;
	    filePath: string;
	    nodeId: string;
	
	    static createFrom(source: any = {}) {
	        return new CardView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.front = source["front"];
	        this.back = source["back"];
	        this.cardType = source["cardType"];
	        this.filePath = source["filePath"];
	        this.nodeId = source["nodeId"];
	    }
	}
	export class Stats {
	    due: number;
	    new: number;
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new Stats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.due = source["due"];
	        this.new = source["new"];
	        this.total = source["total"];
	    }
	}
	export class StudyStats3 {
	    read: number;
	    process: number;
	    recall: number;
	
	    static createFrom(source: any = {}) {
	        return new StudyStats3(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.read = source["read"];
	        this.process = source["process"];
	        this.recall = source["recall"];
	    }
	}
	export class SyncResult {
	    added: number;
	    retired: number;
	    total: number;
	    injectedPaths?: string[];
	    extractsAdded: number;
	    extractsRetired: number;
	
	    static createFrom(source: any = {}) {
	        return new SyncResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.added = source["added"];
	        this.retired = source["retired"];
	        this.total = source["total"];
	        this.injectedPaths = source["injectedPaths"];
	        this.extractsAdded = source["extractsAdded"];
	        this.extractsRetired = source["extractsRetired"];
	    }
	}

}

export namespace llm {
	
	export class Image {
	    mediaType: string;
	    data: string;
	
	    static createFrom(source: any = {}) {
	        return new Image(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mediaType = source["mediaType"];
	        this.data = source["data"];
	    }
	}
	export class Message {
	    role: string;
	    content: string;
	    images?: Image[];
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.images = this.convertValues(source["images"], Image);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {
	
	export class BuildGraphResult {
	    nodesIngested: number;
	    linksIngested: number;
	    filesProcessed: number;
	    durationMs: number;
	    error?: string;
	    errors?: string[];
	    unknownArrows?: Record<string, number>;
	    parseErrors?: content.ParseError[];
	
	    static createFrom(source: any = {}) {
	        return new BuildGraphResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.nodesIngested = source["nodesIngested"];
	        this.linksIngested = source["linksIngested"];
	        this.filesProcessed = source["filesProcessed"];
	        this.durationMs = source["durationMs"];
	        this.error = source["error"];
	        this.errors = source["errors"];
	        this.unknownArrows = source["unknownArrows"];
	        this.parseErrors = this.convertValues(source["parseErrors"], content.ParseError);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SourceInfo {
	    kind: string;
	    anchor: string;
	    passage: string;
	    refPath: string;
	
	    static createFrom(source: any = {}) {
	        return new SourceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.anchor = source["anchor"];
	        this.passage = source["passage"];
	        this.refPath = source["refPath"];
	    }
	}
	export class OrbitItem {
	    arrowLong: string;
	    neighborText: string;
	
	    static createFrom(source: any = {}) {
	        return new OrbitItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.arrowLong = source["arrowLong"];
	        this.neighborText = source["neighborText"];
	    }
	}
	export class ContextPayload {
	    extractText: string;
	    tags: string[];
	    orbit: OrbitItem[];
	    source: SourceInfo;
	
	    static createFrom(source: any = {}) {
	        return new ContextPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.extractText = source["extractText"];
	        this.tags = source["tags"];
	        this.orbit = this.convertValues(source["orbit"], OrbitItem);
	        this.source = this.convertValues(source["source"], SourceInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CreateExtractView {
	    id: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateExtractView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	    }
	}
	export class ExtractTarget {
	    id: string;
	    filePath: string;
	    line: number;
	    text: string;
	    fromAnchor: string;
	
	    static createFrom(source: any = {}) {
	        return new ExtractTarget(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.filePath = source["filePath"];
	        this.line = source["line"];
	        this.text = source["text"];
	        this.fromAnchor = source["fromAnchor"];
	    }
	}
	export class ExtractView {
	    id: string;
	    text: string;
	    fromAnchor: string;
	    filePath: string;
	
	    static createFrom(source: any = {}) {
	        return new ExtractView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.text = source["text"];
	        this.fromAnchor = source["fromAnchor"];
	        this.filePath = source["filePath"];
	    }
	}
	export class LLMProfile {
	    id: string;
	    name: string;
	    kind: string;
	    baseUrl: string;
	    model: string;
	    apiKey?: string;
	    apiKeyEnv?: string;
	
	    static createFrom(source: any = {}) {
	        return new LLMProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.baseUrl = source["baseUrl"];
	        this.model = source["model"];
	        this.apiKey = source["apiKey"];
	        this.apiKeyEnv = source["apiKeyEnv"];
	    }
	}
	export class LLMConfig {
	    profiles: LLMProfile[];
	    activeProfileId: string;
	
	    static createFrom(source: any = {}) {
	        return new LLMConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profiles = this.convertValues(source["profiles"], LLMProfile);
	        this.activeProfileId = source["activeProfileId"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class NotebookLinkMark {
	    line: number;
	    notebookIds: string[];
	
	    static createFrom(source: any = {}) {
	        return new NotebookLinkMark(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.line = source["line"];
	        this.notebookIds = source["notebookIds"];
	    }
	}
	export class NotebookView {
	    notebookId: string;
	    nodeId: string;
	    chapter: string;
	    cells: number[];
	    context: ContextPayload;
	
	    static createFrom(source: any = {}) {
	        return new NotebookView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.notebookId = source["notebookId"];
	        this.nodeId = source["nodeId"];
	        this.chapter = source["chapter"];
	        this.cells = source["cells"];
	        this.context = this.convertValues(source["context"], ContextPayload);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ProviderInfo {
	    id: string;
	    name: string;
	    kinds: string[];
	
	    static createFrom(source: any = {}) {
	        return new ProviderInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kinds = source["kinds"];
	    }
	}
	export class ReadingView {
	    id: string;
	    refPath: string;
	    notesPath: string;
	    title: string;
	    position: string;
	    percent: number;
	    due: number;
	    done: boolean;
	    medium: string;
	    sourceUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new ReadingView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.refPath = source["refPath"];
	        this.notesPath = source["notesPath"];
	        this.title = source["title"];
	        this.position = source["position"];
	        this.percent = source["percent"];
	        this.due = source["due"];
	        this.done = source["done"];
	        this.medium = source["medium"];
	        this.sourceUrl = source["sourceUrl"];
	    }
	}
	export class SaveFileResult {
	    injections: content.IDInjection[];
	    finalContent: string;
	
	    static createFrom(source: any = {}) {
	        return new SaveFileResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.injections = this.convertValues(source["injections"], content.IDInjection);
	        this.finalContent = source["finalContent"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class StartTurnReq {
	    profileId: string;
	    system: string;
	    messages: llm.Message[];
	
	    static createFrom(source: any = {}) {
	        return new StartTurnReq(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.system = source["system"];
	        this.messages = this.convertValues(source["messages"], llm.Message);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace mediasearch {
	
	export class Language {
	    name: string;
	    code: string;
	
	    static createFrom(source: any = {}) {
	        return new Language(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.code = source["code"];
	    }
	}
	export class Result {
	    kind: string;
	    thumbUrl: string;
	    fullUrl: string;
	    title: string;
	    sourcePage: string;
	    license: string;
	    lang: string;
	    speaker: string;
	    mime: string;
	    durationMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.thumbUrl = source["thumbUrl"];
	        this.fullUrl = source["fullUrl"];
	        this.title = source["title"];
	        this.sourcePage = source["sourcePage"];
	        this.license = source["license"];
	        this.lang = source["lang"];
	        this.speaker = source["speaker"];
	        this.mime = source["mime"];
	        this.durationMs = source["durationMs"];
	    }
	}

}

export namespace search {
	
	export class ChainNode {
	    NodeText: string;
	    ArrowCode: string;
	    ArrowLong: string;
	
	    static createFrom(source: any = {}) {
	        return new ChainNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.NodeText = source["NodeText"];
	        this.ArrowCode = source["ArrowCode"];
	        this.ArrowLong = source["ArrowLong"];
	    }
	}
	export class NoteArrow {
	    ArrowLong: string;
	    ArrowCode: string;
	    Target: string;
	
	    static createFrom(source: any = {}) {
	        return new NoteArrow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ArrowLong = source["ArrowLong"];
	        this.ArrowCode = source["ArrowCode"];
	        this.Target = source["Target"];
	    }
	}
	export class NoteBlock {
	    NodeText: string;
	    Line: number;
	    Context: string;
	    Arrows: NoteArrow[];
	
	    static createFrom(source: any = {}) {
	        return new NoteBlock(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.NodeText = source["NodeText"];
	        this.Line = source["Line"];
	        this.Context = source["Context"];
	        this.Arrows = this.convertValues(source["Arrows"], NoteArrow);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FullNote {
	    Chapter: string;
	    Blocks: NoteBlock[];
	
	    static createFrom(source: any = {}) {
	        return new FullNote(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Chapter = source["Chapter"];
	        this.Blocks = this.convertValues(source["Blocks"], NoteBlock);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class OrbitEntry {
	    ArrowLong: string;
	    NeighborText: string;
	    ParentText: string;
	    Radius: number;
	    Ctx: string;
	
	    static createFrom(source: any = {}) {
	        return new OrbitEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ArrowLong = source["ArrowLong"];
	        this.NeighborText = source["NeighborText"];
	        this.ParentText = source["ParentText"];
	        this.Radius = source["Radius"];
	        this.Ctx = source["Ctx"];
	    }
	}
	export class GraphNode {
	    NodeText: string;
	    Chapter: string;
	    ContextTags: string[];
	    Orbit: OrbitEntry[];
	
	    static createFrom(source: any = {}) {
	        return new GraphNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.NodeText = source["NodeText"];
	        this.Chapter = source["Chapter"];
	        this.ContextTags = source["ContextTags"];
	        this.Orbit = this.convertValues(source["Orbit"], OrbitEntry);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ListingItem {
	    Kind: string;
	    Title: string;
	    Detail: string;
	    NodeText: string;
	    Nodes: string[];
	    Contexts: string[];
	    Chapters: string[];
	    MetaType: string;
	    Context: string;
	
	    static createFrom(source: any = {}) {
	        return new ListingItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Kind = source["Kind"];
	        this.Title = source["Title"];
	        this.Detail = source["Detail"];
	        this.NodeText = source["NodeText"];
	        this.Nodes = source["Nodes"];
	        this.Contexts = source["Contexts"];
	        this.Chapters = source["Chapters"];
	        this.MetaType = source["MetaType"];
	        this.Context = source["Context"];
	    }
	}
	
	
	
	export class StructuralChain {
	    Kind: string;
	    Nodes: ChainNode[];
	    Context: string;
	    Chapter: string;
	
	    static createFrom(source: any = {}) {
	        return new StructuralChain(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Kind = source["Kind"];
	        this.Nodes = this.convertValues(source["Nodes"], ChainNode);
	        this.Context = source["Context"];
	        this.Chapter = source["Chapter"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SearchResult {
	    GraphUnavailable: boolean;
	    GraphError: string;
	    Nodes: GraphNode[];
	    Listings: ListingItem[];
	    Structural: StructuralChain[];
	    Notes: FullNote[];
	
	    static createFrom(source: any = {}) {
	        return new SearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.GraphUnavailable = source["GraphUnavailable"];
	        this.GraphError = source["GraphError"];
	        this.Nodes = this.convertValues(source["Nodes"], GraphNode);
	        this.Listings = this.convertValues(source["Listings"], ListingItem);
	        this.Structural = this.convertValues(source["Structural"], StructuralChain);
	        this.Notes = this.convertValues(source["Notes"], FullNote);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

